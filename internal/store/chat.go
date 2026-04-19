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
	DefaultChatAudioDir          = "./audio"
	DefaultChatAudioMaxSeconds   = 60
	DefaultChatAudioMaxMegabytes = 10
)

var CHAT_MAX_MESSAGES = DefaultChatMaxMessages
var CHAT_AUDIO_DIR = DefaultChatAudioDir
var CHAT_AUDIO_MAX_SECONDS = DefaultChatAudioMaxSeconds
var CHAT_AUDIO_MAX_BYTES int64 = DefaultChatAudioMaxMegabytes * 1024 * 1024

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

func (cr *ChatRepository) AddMessage(conversationID, senderEmail, senderLogin, text string) (model.ChatMessage, error) {
	result, err := cr.AddMessageWithResult(conversationID, senderEmail, senderLogin, text)
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

func (cr *ChatRepository) AddMessageWithResult(conversationID, senderEmail, senderLogin, text string) (ChatAddMessageResult, error) {
	if conversationID == "" {
		return ChatAddMessageResult{}, fmt.Errorf("conversation id is required")
	}
	if senderEmail == "" {
		return ChatAddMessageResult{}, fmt.Errorf("sender email is required")
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
			Type:           "text",
			SenderEmail:    senderEmail,
			SenderLogin:    senderLogin,
			Text:           text,
			CreatedAt:      now,
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

func (cr *ChatRepository) ConsumeAudioMessage(conversationID, messageID, email string) (model.ChatMessage, error) {
	var consumed model.ChatMessage
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
		if message.Audio.ConsumedAt != nil {
			return fmt.Errorf("audio already consumed")
		}

		now := time.Now().UTC()
		message.Audio.ConsumedAt = &now
		if err := saveMessage(tx, message); err != nil {
			return err
		}
		consumed = message
		return nil
	})
	return consumed, err
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
			if err := removeMessageAudioFile(message); err != nil {
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
		if err := removeMessageAudioFile(messages[i]); err != nil {
			return nil, err
		}
		if err := tx.Bucket(ChatMessagesBucket).Delete([]byte(messageKey(conversationID, messages[i].ID))); err != nil {
			return nil, err
		}
	}
	if err := repairMemberReadPointersAfterTrim(tx, conversationID); err != nil {
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

var chatIDSeq uint64

func newChatID(prefix string) string {
	seq := atomic.AddUint64(&chatIDSeq, 1)
	return fmt.Sprintf("%s|%020d|%d", prefix, time.Now().UTC().UnixNano(), seq)
}
