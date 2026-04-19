package routes

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/go-www/silverlining"
	"github.com/golang-jwt/jwt/v5"
)

type chatUserDTO struct {
	Login   string `json:"login"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type chatReceiptDTO struct {
	Login string    `json:"login"`
	Email string    `json:"email"`
	At    time.Time `json:"at"`
}

type chatMessageDTO struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversation_id"`
	Type           string           `json:"type"`
	SenderEmail    string           `json:"sender_email"`
	SenderLogin    string           `json:"sender_login"`
	Text           string           `json:"text"`
	CreatedAt      time.Time        `json:"created_at"`
	DeliveredTo    []chatReceiptDTO `json:"delivered_to"`
	ReadBy         []chatReceiptDTO `json:"read_by"`
	Audio          *chatAudioDTO    `json:"audio,omitempty"`
}

type chatAudioDTO struct {
	ID              string `json:"id"`
	MimeType        string `json:"mime_type"`
	SizeBytes       int64  `json:"size_bytes"`
	DurationSeconds int    `json:"duration_seconds"`
	Consumed        bool   `json:"consumed"`
}

type chatMemberDTO struct {
	ConversationID    string    `json:"conversation_id"`
	Email             string    `json:"email"`
	Login             string    `json:"login"`
	JoinedAt          time.Time `json:"joined_at"`
	LastReadMessageID string    `json:"last_read_message_id"`
}

type chatConversationDTO struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Title           string          `json:"title"`
	CreatedByEmail  string          `json:"created_by_email"`
	CreatedByLogin  string          `json:"created_by_login"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	LastMessageID   string          `json:"last_message_id"`
	LastMessageText string          `json:"last_message_text"`
	LastMessageAt   time.Time       `json:"last_message_at"`
	LastMessage     *chatMessageDTO `json:"last_message,omitempty"`
	Members         []chatMemberDTO `json:"members"`
	UnreadCount     int             `json:"unread_count"`
}

type chatDirectBody struct {
	Email string `json:"email"`
}

type chatGroupBody struct {
	Title        string   `json:"title"`
	MemberEmails []string `json:"member_emails"`
}

type chatRenameBody struct {
	Title string `json:"title"`
}

type chatMemberBody struct {
	Emails []string `json:"emails"`
}

func currentChatUser(ctx *silverlining.Context) (model.UserData, error) {
	tokenStr, err := middleware.GetToken(ctx)
	if err != nil {
		return model.UserData{}, err
	}

	token, err := jwt.ParseWithClaims(tokenStr, &middleware.UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return middleware.GetJwt().Key, nil
	})
	if err != nil || !token.Valid {
		return model.UserData{}, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*middleware.UserClaims)
	if !ok {
		return model.UserData{}, errors.New("invalid token")
	}

	user, err := store.GetUserRepository().FindUserByEmail(claims.Email)
	if err != nil {
		return model.UserData{}, err
	}
	return user, nil
}

func chatUserDTOs(users []model.UserData) []chatUserDTO {
	result := make([]chatUserDTO, 0, len(users))
	for _, user := range users {
		result = append(result, chatUserDTO{Login: user.Login, Email: user.Email, IsAdmin: user.IsAdmin})
	}
	return result
}

func ensureGroupMember(currentUser model.UserData, conversationID string) (model.ChatConversation, []model.ChatMember, error) {
	repo := store.GetChatRepository()
	conversation, err := repo.FindConversationByID(conversationID)
	if err != nil {
		return model.ChatConversation{}, nil, err
	}

	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		return model.ChatConversation{}, nil, err
	}
	if _, ok := findMember(members, currentUser.Email); !ok {
		return model.ChatConversation{}, nil, fmt.Errorf("user is not a member of conversation")
	}
	return conversation, members, nil
}

func containsEmail(emails []string, email string) bool {
	for _, item := range emails {
		if item == email {
			return true
		}
	}
	return false
}

func chatMemberDTOs(members []model.ChatMember) []chatMemberDTO {
	result := make([]chatMemberDTO, 0, len(members))
	for _, member := range members {
		result = append(result, chatMemberDTO{
			ConversationID:    member.ConversationID,
			Email:             member.Email,
			Login:             member.Login,
			JoinedAt:          member.JoinedAt,
			LastReadMessageID: member.LastReadMessageID,
		})
	}
	return result
}

func chatMessageDTOFromModel(message model.ChatMessage) chatMessageDTO {
	messageType := message.Type
	if messageType == "" {
		messageType = "text"
	}

	dto := chatMessageDTO{
		ID:             message.ID,
		ConversationID: message.ConversationID,
		Type:           messageType,
		SenderEmail:    message.SenderEmail,
		SenderLogin:    message.SenderLogin,
		Text:           message.Text,
		CreatedAt:      message.CreatedAt,
		DeliveredTo:    chatReceiptDTOs(message.DeliveredTo),
		ReadBy:         chatReceiptDTOs(message.ReadBy),
	}
	if message.Audio != nil {
		dto.Audio = &chatAudioDTO{
			ID:              message.Audio.ID,
			MimeType:        message.Audio.MimeType,
			SizeBytes:       message.Audio.SizeBytes,
			DurationSeconds: message.Audio.DurationSeconds,
			Consumed:        message.Audio.ConsumedAt != nil,
		}
	}
	return dto
}

func chatReceiptDTOs(receipts []model.MessageReceipt) []chatReceiptDTO {
	result := make([]chatReceiptDTO, 0, len(receipts))
	for _, receipt := range receipts {
		result = append(result, chatReceiptDTO{
			Login: receipt.Login,
			Email: receipt.Email,
			At:    receipt.At,
		})
	}
	return result
}

func conversationView(ctx *silverlining.Context, conversationID, currentUserEmail string) (chatConversationDTO, error) {
	repo := store.GetChatRepository()
	conversation, err := repo.FindConversationByID(conversationID)
	if err != nil {
		return chatConversationDTO{}, err
	}

	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		return chatConversationDTO{}, err
	}
	member, ok := findMember(members, currentUserEmail)
	if !ok {
		return chatConversationDTO{}, fmt.Errorf("user is not a member of conversation")
	}

	messages, err := repo.ListMessages(conversationID)
	if err != nil {
		return chatConversationDTO{}, err
	}

	view := chatConversationDTO{
		ID:              conversation.ID,
		Type:            conversation.Type,
		Title:           conversation.Title,
		CreatedByEmail:  conversation.CreatedByEmail,
		CreatedByLogin:  conversation.CreatedByLogin,
		CreatedAt:       conversation.CreatedAt,
		UpdatedAt:       conversation.UpdatedAt,
		LastMessageID:   conversation.LastMessageID,
		LastMessageText: conversation.LastMessageText,
		LastMessageAt:   conversation.LastMessageAt,
		Members:         chatMemberDTOs(members),
		UnreadCount:     unreadCount(messages, member, currentUserEmail),
	}
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		lastMessage := chatMessageDTOFromModel(last)
		view.LastMessage = &lastMessage
	}
	return view, nil
}

func conversationViewsForUser(ctx *silverlining.Context, currentUserEmail string) ([]chatConversationDTO, error) {
	ids, err := store.GetChatRepository().ListUserConversations(currentUserEmail)
	if err != nil {
		return nil, err
	}
	result := make([]chatConversationDTO, 0, len(ids))
	for _, conversationID := range ids {
		view, err := conversationView(ctx, conversationID, currentUserEmail)
		if err != nil {
			continue
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func unreadCount(messages []model.ChatMessage, member model.ChatMember, currentUserEmail string) int {
	if len(messages) == 0 {
		return 0
	}

	lastReadIndex := -1
	if member.LastReadMessageID != "" {
		for i, message := range messages {
			if message.ID == member.LastReadMessageID {
				lastReadIndex = i
				break
			}
		}
	}

	count := 0
	for i := lastReadIndex + 1; i < len(messages); i++ {
		if messages[i].SenderEmail == currentUserEmail {
			continue
		}
		count++
	}
	return count
}

func findMember(members []model.ChatMember, email string) (model.ChatMember, bool) {
	for _, member := range members {
		if member.Email == email {
			return member, true
		}
	}
	return model.ChatMember{}, false
}

func uniqueEmails(emails []string) []string {
	seen := make(map[string]struct{}, len(emails))
	result := make([]string, 0, len(emails))
	for _, email := range emails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		result = append(result, email)
	}
	return result
}

func writeChatError(ctx *silverlining.Context, status int, message string) {
	GetError(ctx, &Error{Message: message, Status: status})
}

func logChatError(err error) {
	if err != nil {
		log.Print(err)
	}
}
