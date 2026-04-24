package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	DefaultChatMaxMessages       = 100
	DefaultChatCallMaxMembers    = 4
	DefaultChatAudioDir          = "./audio"
	DefaultChatAudioMaxSeconds   = 60
	DefaultChatAudioMaxMegabytes = 10
	DefaultChatImageDir          = "./images"
	DefaultChatImageMaxMegabytes = 10
	DefaultChatAudioTTL          = 24 * time.Hour
)

var CHAT_MAX_MESSAGES = DefaultChatMaxMessages
var CHAT_AUDIO_DIR = DefaultChatAudioDir
var CHAT_AUDIO_MAX_SECONDS = DefaultChatAudioMaxSeconds
var CHAT_AUDIO_MAX_BYTES int64 = DefaultChatAudioMaxMegabytes * 1024 * 1024
var CHAT_AUDIO_TTL = DefaultChatAudioTTL
var CHAT_IMAGE_DIR = DefaultChatImageDir
var CHAT_IMAGE_MAX_BYTES int64 = DefaultChatImageMaxMegabytes * 1024 * 1024
var CHAT_IMAGE_TTL = DefaultChatAudioTTL
var CHAT_CALL_MAX_MEMBERS = DefaultChatCallMaxMembers

func ConfigureChatMaxMessages(raw string) {
	if raw == "" {
		CHAT_MAX_MESSAGES = DefaultChatMaxMessages
		return
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 2 {
		CHAT_MAX_MESSAGES = DefaultChatMaxMessages
		return
	}

	CHAT_MAX_MESSAGES = value
}

func ConfigureChatAudio(rawDir, rawSeconds, rawMaxMegabytes string) {
	CHAT_AUDIO_DIR = DefaultChatAudioDir
	if strings.TrimSpace(rawDir) != "" {
		CHAT_AUDIO_DIR = strings.TrimSpace(rawDir)
	}

	CHAT_AUDIO_MAX_SECONDS = DefaultChatAudioMaxSeconds
	if seconds, err := strconv.Atoi(rawSeconds); err == nil && seconds > 0 {
		CHAT_AUDIO_MAX_SECONDS = seconds
	}

	CHAT_AUDIO_MAX_BYTES = DefaultChatAudioMaxMegabytes * 1024 * 1024
	if megabytes, err := strconv.Atoi(rawMaxMegabytes); err == nil && megabytes > 0 {
		CHAT_AUDIO_MAX_BYTES = int64(megabytes) * 1024 * 1024
	}
}

func ConfigureChatImage(rawDir, rawMaxMegabytes string) {
	CHAT_IMAGE_DIR = DefaultChatImageDir
	if strings.TrimSpace(rawDir) != "" {
		CHAT_IMAGE_DIR = strings.TrimSpace(rawDir)
	}

	CHAT_IMAGE_MAX_BYTES = DefaultChatImageMaxMegabytes * 1024 * 1024
	if megabytes, err := strconv.Atoi(rawMaxMegabytes); err == nil && megabytes > 0 {
		CHAT_IMAGE_MAX_BYTES = int64(megabytes) * 1024 * 1024
	}
}

type ChatRepository struct {
	repo *db.Repository
}

func GetChatRepository() *ChatRepository {
	return &ChatRepository{
		repo: db.GetRepository(),
	}
}

func (cr *ChatRepository) CreateDirectConversation(first, second model.ChatMember) (model.ChatConversation, error) {
	if first.Email == "" || second.Email == "" {
		return model.ChatConversation{}, fmt.Errorf("both participants are required")
	}
	if first.Email == second.Email {
		return model.ChatConversation{}, fmt.Errorf("direct conversation requires two different users")
	}

	conv := model.ChatConversation{
		ID:             directConversationID(first.Email, second.Email),
		Type:           "direct",
		CreatedByEmail: first.Email,
		CreatedByLogin: first.Login,
		CreatedAt:      time.Now().UTC(),
	}
	conv.UpdatedAt = conv.CreatedAt

	return cr.upsertConversation(conv, []model.ChatMember{
		normalizeMember(conv.ID, first),
		normalizeMember(conv.ID, second),
	})
}

func (cr *ChatRepository) CreateGroupConversation(title string, members []model.ChatMember) (model.ChatConversation, error) {
	if title == "" {
		return model.ChatConversation{}, fmt.Errorf("group title is required")
	}
	members = uniqueMembers(members)
	if len(members) == 0 {
		return model.ChatConversation{}, fmt.Errorf("group conversation requires members")
	}

	conv := model.ChatConversation{
		ID:             newChatID("group"),
		Type:           "group",
		Title:          title,
		CreatedByEmail: members[0].Email,
		CreatedByLogin: members[0].Login,
		CreatedAt:      time.Now().UTC(),
	}
	conv.UpdatedAt = conv.CreatedAt

	normalized := make([]model.ChatMember, 0, len(members))
	for _, member := range members {
		normalized = append(normalized, normalizeMember(conv.ID, member))
	}

	return cr.upsertConversation(conv, normalized)
}

func (cr *ChatRepository) ListConversations() ([]model.ChatConversation, error) {
	conversations := make([]model.ChatConversation, 0)
	err := cr.repo.View(func(tx *bolt.Tx) error {
		return scanBucket(tx, ChatConversationsBucket, func(_ []byte, data []byte) error {
			var conversation model.ChatConversation
			if err := json.Unmarshal(data, &conversation); err != nil {
				return nil
			}
			conversations = append(conversations, conversation)
			return nil
		})
	})
	sort.Slice(conversations, func(i, j int) bool {
		if conversations[i].CreatedAt.Equal(conversations[j].CreatedAt) {
			return conversations[i].ID < conversations[j].ID
		}
		return conversations[i].CreatedAt.Before(conversations[j].CreatedAt)
	})
	return conversations, err
}

func (cr *ChatRepository) FindConversationByID(conversationID string) (model.ChatConversation, error) {
	var conversation model.ChatConversation
	err := cr.repo.View(func(tx *bolt.Tx) error {
		var err error
		conversation, err = loadConversation(tx, conversationID)
		return err
	})
	return conversation, err
}

func (cr *ChatRepository) ListConversationMembers(conversationID string) ([]model.ChatMember, error) {
	members := make([]model.ChatMember, 0)
	err := cr.repo.View(func(tx *bolt.Tx) error {
		return scanBucket(tx, ChatMembersBucket, func(key []byte, data []byte) error {
			if !strings.HasPrefix(string(key), conversationID+"|") {
				return nil
			}
			var member model.ChatMember
			if err := json.Unmarshal(data, &member); err != nil {
				return nil
			}
			members = append(members, member)
			return nil
		})
	})
	sort.Slice(members, func(i, j int) bool {
		if members[i].JoinedAt.Equal(members[j].JoinedAt) {
			return members[i].Email < members[j].Email
		}
		return members[i].JoinedAt.Before(members[j].JoinedAt)
	})
	return members, err
}

func (cr *ChatRepository) ListUserConversations(email string) ([]string, error) {
	result := make([]string, 0)
	err := cr.repo.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(ChatUserConversationsBucket)
		if b == nil {
			return fmt.Errorf("chat user conversations bucket not found")
		}
		data := b.Get([]byte(email))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &result)
	})
	sort.Strings(result)
	return result, err
}

func (cr *ChatRepository) AddMessage(conversationID, senderEmail, senderLogin, text string, replyToMessageID ...string) (model.ChatMessage, error) {
	result, err := cr.AddMessageWithResult(
		conversationID,
		senderEmail,
		senderLogin,
		text,
		replyToMessageID...,
	)
	return result.Message, err
}

type ChatAddMessageResult struct {
	Message           model.ChatMessage
	RemovedMessageIDs []string
}

type ChatAudioUpload struct {
	FilePath        string
	MimeType        string
	SizeBytes       int64
	DurationSeconds int
}

type ChatImageUpload struct {
	FilePath  string
	MimeType  string
	SizeBytes int64
}

type ChatDeleteMessageResult struct {
	Conversation     model.ChatConversation
	DeletedMessageID string
	AffectedMessages []model.ChatMessage
}

type ChatSearchResult struct {
	ConversationID    string
	ConversationTitle string
	Message           model.ChatMessage
}

func (cr *ChatRepository) StartCall(conversationID, starterEmail, starterLogin string) (model.ChatCall, model.ChatMessage, []string, error) {
	if conversationID == "" {
		return model.ChatCall{}, model.ChatMessage{}, nil, fmt.Errorf("conversation id is required")
	}
	if starterEmail == "" {
		return model.ChatCall{}, model.ChatMessage{}, nil, fmt.Errorf("starter email is required")
	}

	var (
		call       model.ChatCall
		message    model.ChatMessage
		removedIDs []string
	)
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, starterEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", starterEmail, conversationID)
		}
		if _, err := loadActiveCall(tx, conversationID); err == nil {
			return fmt.Errorf("conversation already has an active call")
		}
		if _, err := loadActiveCallForUser(tx, starterEmail); err == nil {
			return fmt.Errorf("user already has an active call")
		}

		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		call = model.ChatCall{
			ID:              newChatID("call"),
			ConversationID:  conversationID,
			MessageID:       newChatID("msg"),
			StartedByEmail:  starterEmail,
			StartedByLogin:  starterLogin,
			StartedAt:       now,
			MaxParticipants: CHAT_CALL_MAX_MEMBERS,
			Participants: []model.ChatCallParticipant{{
				Email:    starterEmail,
				Login:    starterLogin,
				JoinedAt: now,
			}},
		}
		message = callMessageFromCall(call)
		message.ConversationID = conversationID
		message.DeliveredTo = buildDeliveredTo(members, starterEmail, now)

		if err := saveCall(tx, call); err != nil {
			return err
		}
		if err := saveMessage(tx, message); err != nil {
			return err
		}

		conversation.UpdatedAt = now
		conversation.LastMessageID = message.ID
		conversation.LastMessageText = message.Text
		conversation.LastMessageAt = now
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}

		removedIDs, err = trimMessagesWithResult(tx, conversationID)
		return err
	})
	return call, message, removedIDs, err
}

func (cr *ChatRepository) JoinCall(conversationID, callID, email, login string) (model.ChatCall, error) {
	var updated model.ChatCall
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, email) {
			return fmt.Errorf("user %s is not a member of conversation %s", email, conversationID)
		}

		call, err := loadCall(tx, callID)
		if err != nil {
			return err
		}
		if call.ConversationID != conversationID {
			return fmt.Errorf("call does not belong to conversation")
		}
		if call.EndedAt != nil {
			return fmt.Errorf("call already ended")
		}

		if active, err := loadActiveCallForUser(tx, email); err == nil && active.ID != callID {
			return fmt.Errorf("user already has an active call")
		}

		for _, participant := range call.Participants {
			if participant.Email == email {
				updated = call
				return nil
			}
		}
		if len(call.Participants) >= call.MaxParticipants {
			return fmt.Errorf("call capacity exceeded")
		}

		call.Participants = append(call.Participants, model.ChatCallParticipant{
			Email:    email,
			Login:    login,
			JoinedAt: time.Now().UTC(),
		})
		sort.Slice(call.Participants, func(i, j int) bool {
			if call.Participants[i].JoinedAt.Equal(call.Participants[j].JoinedAt) {
				return call.Participants[i].Email < call.Participants[j].Email
			}
			return call.Participants[i].JoinedAt.Before(call.Participants[j].JoinedAt)
		})
		if err := saveCall(tx, call); err != nil {
			return err
		}
		if _, err := syncCallMessage(tx, call); err != nil {
			return err
		}
		updated = call
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) SetCallMuted(conversationID, callID, email string, muted bool) (model.ChatCall, error) {
	var updated model.ChatCall
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		call, err := loadCall(tx, callID)
		if err != nil {
			return err
		}
		if call.ConversationID != conversationID {
			return fmt.Errorf("call does not belong to conversation")
		}
		if call.EndedAt != nil {
			return fmt.Errorf("call already ended")
		}

		found := false
		for i := range call.Participants {
			if call.Participants[i].Email != email {
				continue
			}
			call.Participants[i].Muted = muted
			found = true
			break
		}
		if !found {
			return fmt.Errorf("participant not found")
		}
		if err := saveCall(tx, call); err != nil {
			return err
		}
		updated = call
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) LeaveCall(conversationID, callID, email string) (model.ChatCall, bool, model.ChatMessage, error) {
	var (
		updatedCall    model.ChatCall
		updatedMessage model.ChatMessage
		ended          bool
	)
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		call, err := loadCall(tx, callID)
		if err != nil {
			return err
		}
		if call.ConversationID != conversationID {
			return fmt.Errorf("call does not belong to conversation")
		}
		if call.EndedAt != nil {
			return fmt.Errorf("call already ended")
		}

		filtered := make([]model.ChatCallParticipant, 0, len(call.Participants))
		removed := false
		for _, participant := range call.Participants {
			if participant.Email == email {
				removed = true
				continue
			}
			filtered = append(filtered, participant)
		}
		if !removed {
			return fmt.Errorf("participant not found")
		}

		if len(filtered) == 0 {
			ended = true
			if err := deleteCall(tx, call.ID); err != nil {
				return err
			}
			call.Participants = nil
			now := time.Now().UTC()
			call.EndedAt = &now
			updatedMessage, err = syncCallMessage(tx, call)
			if err != nil {
				return err
			}
			return nil
		}

		call.Participants = filtered
		if err := saveCall(tx, call); err != nil {
			return err
		}
		updatedMessage, err = syncCallMessage(tx, call)
		if err != nil {
			return err
		}
		updatedCall = call
		return nil
	})
	return updatedCall, ended, updatedMessage, err
}

func (cr *ChatRepository) GetActiveCall(conversationID string) (model.ChatCall, error) {
	var call model.ChatCall
	err := cr.repo.View(func(tx *bolt.Tx) error {
		var err error
		call, err = loadActiveCall(tx, conversationID)
		return err
	})
	return call, err
}

func (cr *ChatRepository) EndCall(conversationID, callID string) (model.ChatCall, model.ChatMessage, error) {
	var (
		endedCall  model.ChatCall
		updatedMsg model.ChatMessage
	)
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		call, err := loadCall(tx, callID)
		if err != nil {
			return err
		}
		if call.ConversationID != conversationID {
			return fmt.Errorf("call does not belong to conversation")
		}
		if call.EndedAt != nil {
			return fmt.Errorf("call already ended")
		}

		if err := deleteCall(tx, call.ID); err != nil {
			return err
		}
		now := time.Now().UTC()
		call.EndedAt = &now
		call.Participants = nil

		updatedMsg, err = syncCallMessage(tx, call)
		if err != nil {
			return err
		}
		endedCall = call
		return nil
	})
	return endedCall, updatedMsg, err
}

func (cr *ChatRepository) AddMessageWithResult(conversationID, senderEmail, senderLogin, text string, replyToMessageID ...string) (ChatAddMessageResult, error) {
	if conversationID == "" {
		return ChatAddMessageResult{}, fmt.Errorf("conversation id is required")
	}
	if senderEmail == "" {
		return ChatAddMessageResult{}, fmt.Errorf("sender email is required")
	}
	replyID := ""
	if len(replyToMessageID) > 0 {
		replyID = strings.TrimSpace(replyToMessageID[0])
	}

	var result ChatAddMessageResult
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}

		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}

		if !memberExists(members, senderEmail) {
			return fmt.Errorf("sender %s is not a member of conversation %s", senderEmail, conversationID)
		}

		now := time.Now().UTC()
		message := model.ChatMessage{
			ID:               newChatID("msg"),
			ConversationID:   conversationID,
			Type:             "text",
			SenderEmail:      senderEmail,
			SenderLogin:      senderLogin,
			Text:             text,
			CreatedAt:        now,
			UpdatedAt:        now,
			ReplyToMessageID: replyID,
		}
		message.DeliveredTo = buildDeliveredTo(members, senderEmail, now)

		if err := saveMessage(tx, message); err != nil {
			return err
		}
		conversation.UpdatedAt = now
		conversation.LastMessageID = message.ID
		conversation.LastMessageText = message.Text
		conversation.LastMessageAt = now
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}

		removedIDs, err := trimMessagesWithResult(tx, message.ConversationID)
		if err != nil {
			return err
		}
		result.Message = message
		result.RemovedMessageIDs = removedIDs
		return nil
	})
	return result, err
}

func (cr *ChatRepository) AddAudioMessageWithResult(conversationID, senderEmail, senderLogin string, upload ChatAudioUpload) (ChatAddMessageResult, error) {
	if conversationID == "" {
		return ChatAddMessageResult{}, fmt.Errorf("conversation id is required")
	}
	if senderEmail == "" {
		return ChatAddMessageResult{}, fmt.Errorf("sender email is required")
	}
	if upload.FilePath == "" {
		return ChatAddMessageResult{}, fmt.Errorf("audio file path is required")
	}
	if upload.SizeBytes <= 0 {
		return ChatAddMessageResult{}, fmt.Errorf("audio size is required")
	}
	if upload.DurationSeconds <= 0 {
		return ChatAddMessageResult{}, fmt.Errorf("audio duration is required")
	}

	var result ChatAddMessageResult
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}

		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}

		if !memberExists(members, senderEmail) {
			return fmt.Errorf("sender %s is not a member of conversation %s", senderEmail, conversationID)
		}

		now := time.Now().UTC()
		message := model.ChatMessage{
			ID:             newChatID("msg"),
			ConversationID: conversationID,
			Type:           "audio",
			SenderEmail:    senderEmail,
			SenderLogin:    senderLogin,
			Text:           "Голосовое сообщение",
			CreatedAt:      now,
			Audio: &model.ChatAudio{
				ID:              newChatID("audio"),
				MimeType:        upload.MimeType,
				SizeBytes:       upload.SizeBytes,
				DurationSeconds: upload.DurationSeconds,
				FilePath:        upload.FilePath,
				ExpiresAt:       now.Add(CHAT_AUDIO_TTL),
			},
		}
		message.DeliveredTo = buildDeliveredTo(members, senderEmail, now)

		if err := saveMessage(tx, message); err != nil {
			return err
		}
		conversation.UpdatedAt = now
		conversation.LastMessageID = message.ID
		conversation.LastMessageText = message.Text
		conversation.LastMessageAt = now
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}

		removedIDs, err := trimMessagesWithResult(tx, message.ConversationID)
		if err != nil {
			return err
		}
		result.Message = message
		result.RemovedMessageIDs = removedIDs
		return nil
	})
	return result, err
}

func (cr *ChatRepository) AddImageMessageWithResult(conversationID, senderEmail, senderLogin string, upload ChatImageUpload) (ChatAddMessageResult, error) {
	if conversationID == "" {
		return ChatAddMessageResult{}, fmt.Errorf("conversation id is required")
	}
	if senderEmail == "" {
		return ChatAddMessageResult{}, fmt.Errorf("sender email is required")
	}
	if upload.FilePath == "" {
		return ChatAddMessageResult{}, fmt.Errorf("image file path is required")
	}
	if upload.SizeBytes <= 0 {
		return ChatAddMessageResult{}, fmt.Errorf("image size is required")
	}

	var result ChatAddMessageResult
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}

		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}

		if !memberExists(members, senderEmail) {
			return fmt.Errorf("sender %s is not a member of conversation %s", senderEmail, conversationID)
		}

		now := time.Now().UTC()
		message := model.ChatMessage{
			ID:             newChatID("msg"),
			ConversationID: conversationID,
			Type:           "image",
			SenderEmail:    senderEmail,
			SenderLogin:    senderLogin,
			Text:           "Изображение",
			CreatedAt:      now,
			Image: &model.ChatImage{
				ID:        newChatID("image"),
				MimeType:  upload.MimeType,
				SizeBytes: upload.SizeBytes,
				FilePath:  upload.FilePath,
				ExpiresAt: now.Add(CHAT_IMAGE_TTL),
			},
		}
		message.DeliveredTo = buildDeliveredTo(members, senderEmail, now)

		if err := saveMessage(tx, message); err != nil {
			return err
		}
		conversation.UpdatedAt = now
		conversation.LastMessageID = message.ID
		conversation.LastMessageText = message.Text
		conversation.LastMessageAt = now
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}

		removedIDs, err := trimMessagesWithResult(tx, message.ConversationID)
		if err != nil {
			return err
		}
		result.Message = message
		result.RemovedMessageIDs = removedIDs
		return nil
	})
	return result, err
}

func (cr *ChatRepository) ListMessages(conversationID string) ([]model.ChatMessage, error) {
	var messages []model.ChatMessage
	err := cr.repo.View(func(tx *bolt.Tx) error {
		var err error
		messages, err = loadMessages(tx, conversationID)
		return err
	})
	return messages, err
}

func (cr *ChatRepository) FindMessageForMember(conversationID, messageID, email string) (model.ChatMessage, error) {
	var message model.ChatMessage
	err := cr.repo.View(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, email) {
			return fmt.Errorf("user %s is not a member of conversation %s", email, conversationID)
		}

		loaded, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		message = loaded
		return nil
	})
	return message, err
}

func (cr *ChatRepository) MarkMessageRead(conversationID, messageID, email, login string) error {
	_, err := cr.MarkMessagesReadUpToWithResult(conversationID, messageID, email, login)
	return err
}

func (cr *ChatRepository) UpdateTextMessage(conversationID, messageID, editorEmail, text string) (model.ChatMessage, error) {
	var updated model.ChatMessage
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, editorEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", editorEmail, conversationID)
		}

		message, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if message.Type != "text" {
			return fmt.Errorf("only text messages can be edited")
		}
		if message.SenderEmail != editorEmail {
			return fmt.Errorf("only author can edit message")
		}

		now := time.Now().UTC()
		message.Text = text
		message.UpdatedAt = now
		message.EditedAt = &now
		if err := saveMessage(tx, message); err != nil {
			return err
		}

		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		conversation.UpdatedAt = now
		if conversation.LastMessageID == message.ID {
			conversation.LastMessageText = message.Text
			conversation.LastMessageAt = message.CreatedAt
		}
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}

		updated = message
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) MarkMessageAliceAnnounced(conversationID, messageID string) (model.ChatMessage, error) {
	var updated model.ChatMessage
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		message, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if message.AliceAnnounced {
			updated = message
			return nil
		}

		message.AliceAnnounced = true
		message.UpdatedAt = time.Now().UTC()
		if err := saveMessage(tx, message); err != nil {
			return err
		}
		updated = message
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) DeleteMessage(conversationID, messageID, actorEmail string) (ChatDeleteMessageResult, error) {
	var result ChatDeleteMessageResult
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, actorEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", actorEmail, conversationID)
		}

		message, key, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if err := removeMessageCall(tx, message); err != nil {
			return err
		}
		if err := removeMessageAudioFile(message); err != nil {
			return err
		}
		if err := removeMessageImageFile(message); err != nil {
			return err
		}
		if err := deleteAllMessageReactions(tx, conversationID, messageID); err != nil {
			return err
		}
		if err := tx.Bucket(ChatMessagesBucket).Delete([]byte(key)); err != nil {
			return err
		}

		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		if conversation.PinnedMessageID == messageID {
			conversation.PinnedMessageID = ""
			conversation.PinnedAt = nil
			conversation.PinnedByEmail = ""
			conversation.PinnedByLogin = ""
		}

		now := time.Now().UTC()
		conversation.UpdatedAt = now
		messages, err := loadMessages(tx, conversationID)
		if err != nil {
			return err
		}
		updateConversationLastMessage(&conversation, messages)
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}

		if err := repairMemberReadPointersAfterTrim(tx, conversationID); err != nil {
			return err
		}

		affected := make([]model.ChatMessage, 0)
		for _, candidate := range messages {
			if candidate.ReplyToMessageID != messageID {
				continue
			}
			affected = append(affected, candidate)
		}

		result = ChatDeleteMessageResult{
			Conversation:     conversation,
			DeletedMessageID: messageID,
			AffectedMessages: affected,
		}
		return nil
	})
	return result, err
}

func (cr *ChatRepository) SetMessageReaction(conversationID, messageID, userEmail, userLogin, emoji string) (model.ChatMessage, error) {
	var updated model.ChatMessage
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, userEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", userEmail, conversationID)
		}

		message, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		reaction, err := loadReaction(tx, conversationID, messageID, userEmail)
		if err != nil {
			reaction = model.ChatReaction{
				ConversationID: conversationID,
				MessageID:      messageID,
				UserEmail:      userEmail,
				UserLogin:      userLogin,
				CreatedAt:      now,
			}
		}
		reaction.UserLogin = userLogin
		reaction.Emoji = emoji
		reaction.UpdatedAt = now
		if err := saveReaction(tx, reaction); err != nil {
			return err
		}

		reactions, err := loadMessageReactions(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		message.Reactions = reactions
		updated = message
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) DeleteMessageReaction(conversationID, messageID, userEmail string) (model.ChatMessage, error) {
	var updated model.ChatMessage
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, userEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", userEmail, conversationID)
		}

		message, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if err := deleteReaction(tx, conversationID, messageID, userEmail); err != nil {
			return err
		}
		reactions, err := loadMessageReactions(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		message.Reactions = reactions
		updated = message
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) SetPinnedMessage(conversationID, messageID, userEmail, userLogin string) (model.ChatConversation, error) {
	var updated model.ChatConversation
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, userEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", userEmail, conversationID)
		}
		if _, _, err := loadMessage(tx, conversationID, messageID); err != nil {
			return err
		}

		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		conversation.PinnedMessageID = messageID
		conversation.PinnedAt = &now
		conversation.PinnedByEmail = userEmail
		conversation.PinnedByLogin = userLogin
		conversation.UpdatedAt = now
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}
		updated = conversation
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) ClearPinnedMessage(conversationID, userEmail string) (model.ChatConversation, error) {
	var updated model.ChatConversation
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, userEmail) {
			return fmt.Errorf("user %s is not a member of conversation %s", userEmail, conversationID)
		}

		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		conversation.PinnedMessageID = ""
		conversation.PinnedAt = nil
		conversation.PinnedByEmail = ""
		conversation.PinnedByLogin = ""
		conversation.UpdatedAt = time.Now().UTC()
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}
		updated = conversation
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) SearchTextMessagesForUser(email, query string) ([]ChatSearchResult, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return []ChatSearchResult{}, nil
	}

	conversationIDs, err := cr.ListUserConversations(email)
	if err != nil {
		return nil, err
	}
	results := make([]ChatSearchResult, 0)
	err = cr.repo.View(func(tx *bolt.Tx) error {
		for _, conversationID := range conversationIDs {
			conversation, err := loadConversation(tx, conversationID)
			if err != nil {
				return err
			}
			messages, err := loadMessages(tx, conversationID)
			if err != nil {
				return err
			}
			for _, message := range messages {
				if message.Type != "text" {
					continue
				}
				if !strings.Contains(strings.ToLower(message.Text), query) {
					continue
				}
				results = append(results, ChatSearchResult{
					ConversationID:    conversationID,
					ConversationTitle: conversation.Title,
					Message:           message,
				})
			}
		}
		return nil
	})
	return results, err
}

func (cr *ChatRepository) MarkMessagesReadUpTo(conversationID, messageID, email, login string) error {
	_, err := cr.MarkMessagesReadUpToWithResult(conversationID, messageID, email, login)
	return err
}

func (cr *ChatRepository) MarkMessagesReadUpToWithResult(conversationID, messageID, email, login string) (bool, error) {
	var changed bool
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, email) {
			return fmt.Errorf("reader %s is not a member of conversation %s", email, conversationID)
		}

		messages, err := loadMessages(tx, conversationID)
		if err != nil {
			return err
		}

		targetIndex := -1
		for i, message := range messages {
			if message.ID == messageID {
				targetIndex = i
				break
			}
		}
		if targetIndex == -1 {
			return fmt.Errorf("message not found")
		}

		member, err := loadConversationMember(tx, conversationID, email)
		if err != nil {
			return err
		}

		currentReadIndex := -1
		if member.LastReadMessageID != "" {
			for i, message := range messages {
				if message.ID == member.LastReadMessageID {
					currentReadIndex = i
					break
				}
			}
		}
		if currentReadIndex >= targetIndex && currentReadIndex != -1 {
			return nil
		}

		now := time.Now().UTC()
		for i := 0; i <= targetIndex; i++ {
			message := messages[i]
			if receiptExists(message.ReadBy, email) {
				continue
			}
			message.ReadBy = append(message.ReadBy, model.MessageReceipt{
				Email: email,
				Login: login,
				At:    now,
			})
			if err := saveMessage(tx, message); err != nil {
				return err
			}
			changed = true
		}

		if member.LastReadMessageID != messageID {
			member.LastReadMessageID = messageID
			changed = true
			if err := saveMember(tx, member); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

func (cr *ChatRepository) ConsumeAudioMessage(conversationID, messageID, email, login string) (model.ChatMessage, error) {
	var consumed model.ChatMessage
	var consumeErr error
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, email) {
			return fmt.Errorf("user %s is not a member of conversation %s", email, conversationID)
		}

		message, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if message.Type != "audio" || message.Audio == nil {
			return fmt.Errorf("message is not an audio message")
		}
		now := time.Now().UTC()
		if audioExpired(message.Audio, now) {
			if err := expireAudioMessage(tx, message, now); err != nil {
				return err
			}
			consumeErr = fmt.Errorf("audio expired")
			return nil
		}
		if message.Audio.ConsumedAt != nil {
			return fmt.Errorf("audio already consumed")
		}

		message.Audio.ConsumedAt = &now
		message.Audio.ConsumedByEmail = email
		message.Audio.ConsumedByLogin = login
		message.Audio.FilePath = ""
		if err := saveMessage(tx, message); err != nil {
			return err
		}
		consumed = message
		return nil
	})
	if err != nil {
		return consumed, err
	}
	return consumed, consumeErr
}

func (cr *ChatRepository) CleanupExpiredAudioMessages() ([]string, error) {
	expiredIDs := make([]string, 0)
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		now := time.Now().UTC()
		return scanBucket(tx, ChatMessagesBucket, func(_ []byte, data []byte) error {
			var message model.ChatMessage
			if err := json.Unmarshal(data, &message); err != nil {
				return nil
			}
			if message.Type != "audio" || message.Audio == nil {
				return nil
			}
			if !audioExpired(message.Audio, now) {
				return nil
			}
			if err := expireAudioMessage(tx, message, now); err != nil {
				return err
			}
			expiredIDs = append(expiredIDs, message.ID)
			return nil
		})
	})
	return expiredIDs, err
}

func (cr *ChatRepository) ConsumeImageMessage(conversationID, messageID, email, login string) (model.ChatMessage, error) {
	var consumed model.ChatMessage
	var consumeErr error
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		if !memberExists(members, email) {
			return fmt.Errorf("user %s is not a member of conversation %s", email, conversationID)
		}

		message, _, err := loadMessage(tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if message.Type != "image" || message.Image == nil {
			return fmt.Errorf("message is not an image message")
		}
		now := time.Now().UTC()
		if imageExpired(message.Image, now) {
			if err := expireImageMessage(tx, message, now); err != nil {
				return err
			}
			consumeErr = fmt.Errorf("image expired")
			return nil
		}
		if message.Image.ConsumedAt != nil {
			return fmt.Errorf("image already consumed")
		}

		message.Image.ConsumedAt = &now
		message.Image.ConsumedByEmail = email
		message.Image.ConsumedByLogin = login
		message.Image.FilePath = ""
		if err := saveMessage(tx, message); err != nil {
			return err
		}
		consumed = message
		return nil
	})
	if err != nil {
		return consumed, err
	}
	return consumed, consumeErr
}

func (cr *ChatRepository) ListMessageReactions(conversationID, messageID string) ([]model.ChatReaction, error) {
	var reactions []model.ChatReaction
	err := cr.repo.View(func(tx *bolt.Tx) error {
		var err error
		reactions, err = loadMessageReactions(tx, conversationID, messageID)
		return err
	})
	return reactions, err
}

func (cr *ChatRepository) CleanupExpiredImageMessages() ([]string, error) {
	expiredIDs := make([]string, 0)
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		now := time.Now().UTC()
		return scanBucket(tx, ChatMessagesBucket, func(_ []byte, data []byte) error {
			var message model.ChatMessage
			if err := json.Unmarshal(data, &message); err != nil {
				return nil
			}
			if message.Type != "image" || message.Image == nil {
				return nil
			}
			if !imageExpired(message.Image, now) {
				return nil
			}
			if err := expireImageMessage(tx, message, now); err != nil {
				return err
			}
			expiredIDs = append(expiredIDs, message.ID)
			return nil
		})
	})
	return expiredIDs, err
}

func (cr *ChatRepository) RenameGroupConversation(conversationID, title string) (model.ChatConversation, error) {
	var updated model.ChatConversation
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		if conversation.Type != "group" {
			return fmt.Errorf("conversation is not a group")
		}

		conversation.Title = title
		conversation.UpdatedAt = time.Now().UTC()
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}
		updated = conversation
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) AddGroupMembers(conversationID string, members []model.ChatMember) (model.ChatConversation, error) {
	members = uniqueMembers(members)

	var updated model.ChatConversation
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		if conversation.Type != "group" {
			return fmt.Errorf("conversation is not a group")
		}

		existingMembers, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		existingByEmail := make(map[string]model.ChatMember, len(existingMembers))
		for _, member := range existingMembers {
			existingByEmail[member.Email] = member
		}

		now := time.Now().UTC()
		for _, member := range members {
			if existing, ok := existingByEmail[member.Email]; ok {
				member = existing
			} else {
				member = normalizeMember(conversationID, member)
			}
			if err := saveMember(tx, member); err != nil {
				return err
			}
			if err := addUserConversation(tx, member.Email, conversationID); err != nil {
				return err
			}
		}

		conversation.UpdatedAt = now
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}
		updated = conversation
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) RemoveGroupMembers(conversationID string, emails []string) (model.ChatConversation, error) {
	emailSet := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		emailSet[email] = struct{}{}
	}

	var updated model.ChatConversation
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		if conversation.Type != "group" {
			return fmt.Errorf("conversation is not a group")
		}

		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		for _, member := range members {
			if _, ok := emailSet[member.Email]; !ok {
				continue
			}
			if err := tx.Bucket(ChatMembersBucket).Delete([]byte(memberKey(conversationID, member.Email))); err != nil {
				return err
			}
			if err := removeUserConversation(tx, member.Email, conversationID); err != nil {
				return err
			}
		}

		conversation.UpdatedAt = time.Now().UTC()
		if err := saveConversation(tx, conversation); err != nil {
			return err
		}
		updated = conversation
		return nil
	})
	return updated, err
}

func (cr *ChatRepository) DeleteGroupConversation(conversationID string) error {
	return cr.repo.Update(func(tx *bolt.Tx) error {
		conversation, err := loadConversation(tx, conversationID)
		if err != nil {
			return err
		}
		if conversation.Type != "group" {
			return fmt.Errorf("conversation is not a group")
		}

		members, err := loadConversationMembers(tx, conversationID)
		if err != nil {
			return err
		}
		messages, err := loadMessages(tx, conversationID)
		if err != nil {
			return err
		}

		for _, message := range messages {
			if err := removeMessageCall(tx, message); err != nil {
				return err
			}
			if err := removeMessageAudioFile(message); err != nil {
				return err
			}
			if err := removeMessageImageFile(message); err != nil {
				return err
			}
			if err := deleteAllMessageReactions(tx, conversationID, message.ID); err != nil {
				return err
			}
			if err := tx.Bucket(ChatMessagesBucket).Delete([]byte(messageKey(conversationID, message.ID))); err != nil {
				return err
			}
		}
		for _, member := range members {
			if err := tx.Bucket(ChatMembersBucket).Delete([]byte(memberKey(conversationID, member.Email))); err != nil {
				return err
			}
			if err := removeUserConversation(tx, member.Email, conversationID); err != nil {
				return err
			}
		}

		return tx.Bucket(ChatConversationsBucket).Delete([]byte(conversationID))
	})
}

func (cr *ChatRepository) ClearAll() error {
	if err := cr.repo.ClearBucket(ChatConversationsBucket); err != nil {
		return err
	}
	if err := cr.repo.ClearBucket(ChatMembersBucket); err != nil {
		return err
	}
	if err := cr.repo.ClearBucket(ChatMessagesBucket); err != nil {
		return err
	}
	if err := cr.repo.ClearBucket(ChatCallsBucket); err != nil {
		return err
	}
	if err := cr.repo.ClearBucket(ChatReactionsBucket); err != nil {
		return err
	}
	return cr.repo.ClearBucket(ChatUserConversationsBucket)
}

func (cr *ChatRepository) upsertConversation(conv model.ChatConversation, members []model.ChatMember) (model.ChatConversation, error) {
	err := cr.repo.Update(func(tx *bolt.Tx) error {
		existing, err := loadConversation(tx, conv.ID)
		if err == nil {
			conv = existing
		} else if err := saveConversation(tx, conv); err != nil {
			return err
		}

		for _, member := range members {
			if existingMember, err := loadConversationMember(tx, member.ConversationID, member.Email); err == nil {
				member = existingMember
			}
			if err := saveMember(tx, member); err != nil {
				return err
			}
			if err := addUserConversation(tx, member.Email, conv.ID); err != nil {
				return err
			}
		}
		return nil
	})
	return conv, err
}

func saveConversation(tx *bolt.Tx, conversation model.ChatConversation) error {
	return putJSON(tx.Bucket(ChatConversationsBucket), []byte(conversation.ID), conversation)
}

func saveMember(tx *bolt.Tx, member model.ChatMember) error {
	return putJSON(tx.Bucket(ChatMembersBucket), []byte(memberKey(member.ConversationID, member.Email)), member)
}

func saveMessage(tx *bolt.Tx, message model.ChatMessage) error {
	return putJSON(tx.Bucket(ChatMessagesBucket), []byte(messageKey(message.ConversationID, message.ID)), message)
}

func saveCall(tx *bolt.Tx, call model.ChatCall) error {
	return putJSON(tx.Bucket(ChatCallsBucket), []byte(call.ID), call)
}

func saveReaction(tx *bolt.Tx, reaction model.ChatReaction) error {
	return putJSON(tx.Bucket(ChatReactionsBucket), []byte(reactionKey(reaction.ConversationID, reaction.MessageID, reaction.UserEmail)), reaction)
}

func addUserConversation(tx *bolt.Tx, email, conversationID string) error {
	b := tx.Bucket(ChatUserConversationsBucket)
	if b == nil {
		return fmt.Errorf("chat user conversations bucket not found")
	}

	conversations := make([]string, 0)
	if data := b.Get([]byte(email)); data != nil {
		if err := json.Unmarshal(data, &conversations); err != nil {
			return err
		}
	}

	if !containsString(conversations, conversationID) {
		conversations = append(conversations, conversationID)
		sort.Strings(conversations)
	}
	return putJSON(b, []byte(email), conversations)
}

func removeUserConversation(tx *bolt.Tx, email, conversationID string) error {
	b := tx.Bucket(ChatUserConversationsBucket)
	if b == nil {
		return fmt.Errorf("chat user conversations bucket not found")
	}

	conversations := make([]string, 0)
	if data := b.Get([]byte(email)); data != nil {
		if err := json.Unmarshal(data, &conversations); err != nil {
			return err
		}
	}

	filtered := make([]string, 0, len(conversations))
	for _, item := range conversations {
		if item == conversationID {
			continue
		}
		filtered = append(filtered, item)
	}
	return putJSON(b, []byte(email), filtered)
}

func trimMessages(tx *bolt.Tx, conversationID string) error {
	_, err := trimMessagesWithResult(tx, conversationID)
	return err
}

func trimMessagesWithResult(tx *bolt.Tx, conversationID string) ([]string, error) {
	messages, err := loadMessages(tx, conversationID)
	if err != nil {
		return nil, err
	}

	if len(messages) < CHAT_MAX_MESSAGES {
		return nil, nil
	}

	removeCount := CHAT_MAX_MESSAGES / 2
	if removeCount == 0 {
		return nil, nil
	}

	removedIDs := make([]string, 0, removeCount)
	for i := 0; i < removeCount && i < len(messages); i++ {
		removedIDs = append(removedIDs, messages[i].ID)
		if err := removeMessageCall(tx, messages[i]); err != nil {
			return nil, err
		}
		if err := removeMessageAudioFile(messages[i]); err != nil {
			return nil, err
		}
		if err := removeMessageImageFile(messages[i]); err != nil {
			return nil, err
		}
		if err := deleteAllMessageReactions(tx, conversationID, messages[i].ID); err != nil {
			return nil, err
		}
		if err := tx.Bucket(ChatMessagesBucket).Delete([]byte(messageKey(conversationID, messages[i].ID))); err != nil {
			return nil, err
		}
	}
	if err := repairMemberReadPointersAfterTrim(tx, conversationID); err != nil {
		return nil, err
	}
	conversation, err := loadConversation(tx, conversationID)
	if err != nil {
		return nil, err
	}
	remaining, err := loadMessages(tx, conversationID)
	if err != nil {
		return nil, err
	}
	updateConversationLastMessage(&conversation, remaining)
	if err := saveConversation(tx, conversation); err != nil {
		return nil, err
	}
	return removedIDs, nil
}

func loadConversation(tx *bolt.Tx, conversationID string) (model.ChatConversation, error) {
	b := tx.Bucket(ChatConversationsBucket)
	if b == nil {
		return model.ChatConversation{}, fmt.Errorf("chat conversations bucket not found")
	}
	data := b.Get([]byte(conversationID))
	if data == nil {
		return model.ChatConversation{}, fmt.Errorf("conversation not found")
	}
	var conversation model.ChatConversation
	if err := json.Unmarshal(data, &conversation); err != nil {
		return model.ChatConversation{}, err
	}
	return conversation, nil
}

func loadConversationMembers(tx *bolt.Tx, conversationID string) ([]model.ChatMember, error) {
	members := make([]model.ChatMember, 0)
	err := scanBucket(tx, ChatMembersBucket, func(key []byte, data []byte) error {
		if !strings.HasPrefix(string(key), conversationID+"|") {
			return nil
		}
		var member model.ChatMember
		if err := json.Unmarshal(data, &member); err != nil {
			return nil
		}
		members = append(members, member)
		return nil
	})
	return members, err
}

func loadConversationMember(tx *bolt.Tx, conversationID, email string) (model.ChatMember, error) {
	b := tx.Bucket(ChatMembersBucket)
	if b == nil {
		return model.ChatMember{}, fmt.Errorf("chat members bucket not found")
	}

	key := memberKey(conversationID, email)
	data := b.Get([]byte(key))
	if data == nil {
		return model.ChatMember{}, fmt.Errorf("member not found")
	}

	var member model.ChatMember
	if err := json.Unmarshal(data, &member); err != nil {
		return model.ChatMember{}, err
	}
	return member, nil
}

func loadMessages(tx *bolt.Tx, conversationID string) ([]model.ChatMessage, error) {
	messages := make([]model.ChatMessage, 0)
	err := scanBucket(tx, ChatMessagesBucket, func(key []byte, data []byte) error {
		if !strings.HasPrefix(string(key), conversationID+"|") {
			return nil
		}
		var message model.ChatMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil
		}
		messages = append(messages, message)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
	return messages, nil
}

func repairMemberReadPointersAfterTrim(tx *bolt.Tx, conversationID string) error {
	members, err := loadConversationMembers(tx, conversationID)
	if err != nil {
		return err
	}
	messages, err := loadMessages(tx, conversationID)
	if err != nil {
		return err
	}

	surviving := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		surviving[message.ID] = struct{}{}
	}

	for _, member := range members {
		if member.LastReadMessageID == "" {
			continue
		}
		if _, ok := surviving[member.LastReadMessageID]; ok {
			continue
		}
		member.LastReadMessageID = ""
		if err := saveMember(tx, member); err != nil {
			return err
		}
	}
	return nil
}

func updateConversationLastMessage(conversation *model.ChatConversation, messages []model.ChatMessage) {
	if conversation == nil {
		return
	}
	if len(messages) == 0 {
		conversation.LastMessageID = ""
		conversation.LastMessageText = ""
		conversation.LastMessageAt = time.Time{}
		return
	}

	last := messages[len(messages)-1]
	conversation.LastMessageID = last.ID
	conversation.LastMessageText = last.Text
	conversation.LastMessageAt = last.CreatedAt
}

func loadMessage(tx *bolt.Tx, conversationID, messageID string) (model.ChatMessage, string, error) {
	b := tx.Bucket(ChatMessagesBucket)
	if b == nil {
		return model.ChatMessage{}, "", fmt.Errorf("chat messages bucket not found")
	}

	key := messageKey(conversationID, messageID)
	data := b.Get([]byte(key))
	if data == nil {
		return model.ChatMessage{}, "", fmt.Errorf("message not found")
	}

	var message model.ChatMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return model.ChatMessage{}, "", err
	}
	return message, key, nil
}

func loadCall(tx *bolt.Tx, callID string) (model.ChatCall, error) {
	b := tx.Bucket(ChatCallsBucket)
	if b == nil {
		return model.ChatCall{}, fmt.Errorf("chat calls bucket not found")
	}

	data := b.Get([]byte(callID))
	if data == nil {
		return model.ChatCall{}, fmt.Errorf("call not found")
	}

	var call model.ChatCall
	if err := json.Unmarshal(data, &call); err != nil {
		return model.ChatCall{}, err
	}
	return call, nil
}

func loadActiveCall(tx *bolt.Tx, conversationID string) (model.ChatCall, error) {
	var result model.ChatCall
	err := scanBucket(tx, ChatCallsBucket, func(_ []byte, data []byte) error {
		var call model.ChatCall
		if err := json.Unmarshal(data, &call); err != nil {
			return nil
		}
		if call.ConversationID != conversationID || call.EndedAt != nil {
			return nil
		}
		result = call
		return fmt.Errorf("found")
	})
	if err != nil && err.Error() == "found" {
		return result, nil
	}
	if err != nil {
		return model.ChatCall{}, err
	}
	return model.ChatCall{}, fmt.Errorf("active call not found")
}

func loadActiveCallForUser(tx *bolt.Tx, email string) (model.ChatCall, error) {
	var result model.ChatCall
	err := scanBucket(tx, ChatCallsBucket, func(_ []byte, data []byte) error {
		var call model.ChatCall
		if err := json.Unmarshal(data, &call); err != nil {
			return nil
		}
		if call.EndedAt != nil {
			return nil
		}
		for _, participant := range call.Participants {
			if participant.Email != email {
				continue
			}
			result = call
			return fmt.Errorf("found")
		}
		return nil
	})
	if err != nil && err.Error() == "found" {
		return result, nil
	}
	if err != nil {
		return model.ChatCall{}, err
	}
	return model.ChatCall{}, fmt.Errorf("active call not found")
}

func deleteCall(tx *bolt.Tx, callID string) error {
	b := tx.Bucket(ChatCallsBucket)
	if b == nil {
		return fmt.Errorf("chat calls bucket not found")
	}
	return b.Delete([]byte(callID))
}

func syncCallMessage(tx *bolt.Tx, call model.ChatCall) (model.ChatMessage, error) {
	message, _, err := loadMessage(tx, call.ConversationID, call.MessageID)
	if err != nil {
		return model.ChatMessage{}, err
	}
	if message.Type != "call" || message.Call == nil {
		return model.ChatMessage{}, fmt.Errorf("message is not a call message")
	}
	message.Call.ParticipantCount = len(call.Participants)
	message.Call.Joinable = call.EndedAt == nil
	message.Call.EndedAt = call.EndedAt
	message.UpdatedAt = time.Now().UTC()
	if err := saveMessage(tx, message); err != nil {
		return model.ChatMessage{}, err
	}

	conversation, err := loadConversation(tx, call.ConversationID)
	if err != nil {
		return model.ChatMessage{}, err
	}
	conversation.UpdatedAt = message.UpdatedAt
	if conversation.LastMessageID == message.ID {
		conversation.LastMessageText = message.Text
		conversation.LastMessageAt = message.CreatedAt
		if err := saveConversation(tx, conversation); err != nil {
			return model.ChatMessage{}, err
		}
	}
	return message, nil
}

func loadMessageReactions(tx *bolt.Tx, conversationID, messageID string) ([]model.ChatReaction, error) {
	reactions := make([]model.ChatReaction, 0)
	err := scanBucket(tx, ChatReactionsBucket, func(key []byte, data []byte) error {
		prefix := reactionPrefix(conversationID, messageID)
		if !strings.HasPrefix(string(key), prefix) {
			return nil
		}
		var reaction model.ChatReaction
		if err := json.Unmarshal(data, &reaction); err != nil {
			return nil
		}
		reactions = append(reactions, reaction)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(reactions, func(i, j int) bool {
		if reactions[i].UpdatedAt.Equal(reactions[j].UpdatedAt) {
			return reactions[i].UserEmail < reactions[j].UserEmail
		}
		return reactions[i].UpdatedAt.Before(reactions[j].UpdatedAt)
	})
	return reactions, nil
}

func callMessageFromCall(call model.ChatCall) model.ChatMessage {
	return model.ChatMessage{
		ID:             call.MessageID,
		ConversationID: call.ConversationID,
		Type:           "call",
		SenderEmail:    call.StartedByEmail,
		SenderLogin:    call.StartedByLogin,
		Text:           "Начался звонок",
		CreatedAt:      call.StartedAt,
		UpdatedAt:      call.StartedAt,
		Call: &model.ChatCallMessage{
			CallID:           call.ID,
			StartedByEmail:   call.StartedByEmail,
			StartedByLogin:   call.StartedByLogin,
			StartedAt:        call.StartedAt,
			Joinable:         call.EndedAt == nil,
			EndedAt:          call.EndedAt,
			ParticipantCount: len(call.Participants),
		},
	}
}

func removeMessageCall(tx *bolt.Tx, message model.ChatMessage) error {
	if message.Call == nil || message.Call.CallID == "" {
		return nil
	}
	return deleteCall(tx, message.Call.CallID)
}

func loadReaction(tx *bolt.Tx, conversationID, messageID, userEmail string) (model.ChatReaction, error) {
	b := tx.Bucket(ChatReactionsBucket)
	if b == nil {
		return model.ChatReaction{}, fmt.Errorf("chat reactions bucket not found")
	}

	data := b.Get([]byte(reactionKey(conversationID, messageID, userEmail)))
	if data == nil {
		return model.ChatReaction{}, fmt.Errorf("reaction not found")
	}
	var reaction model.ChatReaction
	if err := json.Unmarshal(data, &reaction); err != nil {
		return model.ChatReaction{}, err
	}
	return reaction, nil
}

func deleteReaction(tx *bolt.Tx, conversationID, messageID, userEmail string) error {
	b := tx.Bucket(ChatReactionsBucket)
	if b == nil {
		return fmt.Errorf("chat reactions bucket not found")
	}
	return b.Delete([]byte(reactionKey(conversationID, messageID, userEmail)))
}

func deleteAllMessageReactions(tx *bolt.Tx, conversationID, messageID string) error {
	b := tx.Bucket(ChatReactionsBucket)
	if b == nil {
		return fmt.Errorf("chat reactions bucket not found")
	}
	prefix := reactionPrefix(conversationID, messageID)
	toDelete := make([][]byte, 0)
	if err := scanBucket(tx, ChatReactionsBucket, func(key []byte, _ []byte) error {
		if strings.HasPrefix(string(key), prefix) {
			toDelete = append(toDelete, append([]byte(nil), key...))
		}
		return nil
	}); err != nil {
		return err
	}
	for _, key := range toDelete {
		if err := b.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func putJSON(bucket *bolt.Bucket, key []byte, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return bucket.Put(key, data)
}

func scanBucket(tx *bolt.Tx, bucketName []byte, fn func([]byte, []byte) error) error {
	b := tx.Bucket(bucketName)
	if b == nil {
		return fmt.Errorf("bucket %s not found", string(bucketName))
	}

	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func normalizeMember(conversationID string, member model.ChatMember) model.ChatMember {
	member.ConversationID = conversationID
	member.JoinedAt = time.Now().UTC()
	member.LastReadMessageID = ""
	return member
}

func buildDeliveredTo(members []model.ChatMember, senderEmail string, at time.Time) []model.MessageReceipt {
	result := make([]model.MessageReceipt, 0, len(members))
	for _, member := range members {
		if member.Email == senderEmail {
			continue
		}
		result = append(result, model.MessageReceipt{
			Email: member.Email,
			Login: member.Login,
			At:    at,
		})
	}
	return result
}

func memberExists(members []model.ChatMember, email string) bool {
	for _, member := range members {
		if member.Email == email {
			return true
		}
	}
	return false
}

func receiptExists(receipts []model.MessageReceipt, email string) bool {
	for _, receipt := range receipts {
		if receipt.Email == email {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func uniqueMembers(members []model.ChatMember) []model.ChatMember {
	seen := make(map[string]struct{}, len(members))
	result := make([]model.ChatMember, 0, len(members))
	for _, member := range members {
		if _, ok := seen[member.Email]; ok {
			continue
		}
		seen[member.Email] = struct{}{}
		result = append(result, member)
	}
	return result
}

func removeMessageAudioFile(message model.ChatMessage) error {
	if message.Audio == nil || message.Audio.FilePath == "" {
		return nil
	}
	if err := os.Remove(message.Audio.FilePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeMessageImageFile(message model.ChatMessage) error {
	if message.Image == nil || message.Image.FilePath == "" {
		return nil
	}
	if err := os.Remove(message.Image.FilePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func audioExpired(audio *model.ChatAudio, now time.Time) bool {
	if audio == nil {
		return false
	}
	if audio.ExpiredAt != nil {
		return true
	}
	if audio.ConsumedAt != nil {
		return false
	}
	return !audio.ExpiresAt.IsZero() && !audio.ExpiresAt.After(now)
}

func imageExpired(image *model.ChatImage, now time.Time) bool {
	if image == nil {
		return false
	}
	if image.ExpiredAt != nil {
		return true
	}
	if image.ConsumedAt != nil {
		return false
	}
	return !image.ExpiresAt.IsZero() && !image.ExpiresAt.After(now)
}

func expireAudioMessage(tx *bolt.Tx, message model.ChatMessage, now time.Time) error {
	if message.Audio == nil {
		return nil
	}
	if message.Audio.ExpiredAt != nil {
		return nil
	}
	if err := removeMessageAudioFile(message); err != nil {
		return err
	}
	message.Audio.ExpiredAt = &now
	message.Audio.FilePath = ""
	return saveMessage(tx, message)
}

func expireImageMessage(tx *bolt.Tx, message model.ChatMessage, now time.Time) error {
	if message.Image == nil {
		return nil
	}
	if message.Image.ExpiredAt != nil {
		return nil
	}
	if err := removeMessageImageFile(message); err != nil {
		return err
	}
	message.Image.ExpiredAt = &now
	message.Image.FilePath = ""
	return saveMessage(tx, message)
}

func directConversationID(firstEmail, secondEmail string) string {
	emails := []string{firstEmail, secondEmail}
	sort.Strings(emails)
	return "direct|" + strings.Join(emails, "|")
}

func memberKey(conversationID, email string) string {
	return conversationID + "|" + email
}

func messageKey(conversationID, messageID string) string {
	return conversationID + "|" + messageID
}

func reactionPrefix(conversationID, messageID string) string {
	return conversationID + "|" + messageID + "|"
}

func reactionKey(conversationID, messageID, userEmail string) string {
	return reactionPrefix(conversationID, messageID) + userEmail
}

var chatIDSeq uint64

func newChatID(prefix string) string {
	seq := atomic.AddUint64(&chatIDSeq, 1)
	return fmt.Sprintf("%s|%020d|%d", prefix, time.Now().UTC().UnixNano(), seq)
}
