package event

import (
	"botDashboard/internal/alice"
	"botDashboard/internal/model"
	"botDashboard/internal/push"
	"botDashboard/internal/store"
	"log"
	"strings"
)

func HandleChatMessageSendCommand(cmd ChatMessageSendCommand) {
	repo := store.GetChatRepository()
	conversationID := cmd.ConversationID

	if conversationID == "" {
		if cmd.RecipientEmail == "" {
			log.Printf("chat send rejected: recipient email is required for direct conversation")
			return
		}
		recipient, err := store.GetUserRepository().FindUserByEmail(cmd.RecipientEmail)
		if err != nil {
			log.Printf("chat send rejected: %v", err)
			return
		}
		conversation, err := repo.CreateDirectConversation(
			model.ChatMember{Email: cmd.SenderEmail, Login: cmd.SenderLogin},
			model.ChatMember{Email: recipient.Email, Login: recipient.Login},
		)
		if err != nil {
			log.Printf("chat send rejected: %v", err)
			return
		}
		conversationID = conversation.ID
	}

	result, err := repo.AddMessageWithResult(
		conversationID,
		cmd.SenderEmail,
		cmd.SenderLogin,
		cmd.Text,
		cmd.ReplyToMessageID,
	)
	if err != nil {
		log.Printf("chat send rejected: %v", err)
		return
	}

	conversation, members, err := loadChatSnapshot(conversationID)
	if err != nil {
		log.Printf("chat send snapshot failed: %v", err)
		return
	}
	result.Message = announceOnAliceIfRequested(cmd, conversation, members, result.Message)
	if err := PublishChatMessagePersistedEvent(ChatMessagePersistedEvent{
		Conversation: conversation,
		Members:      members,
		Message:      result.Message,
	}); err != nil {
		log.Printf("chat persisted publish failed: %v", err)
	}
	push.NotifyChatMembersAboutMessage(conversation, members, result.Message)
	if len(result.RemovedMessageIDs) > 0 {
		if err := PublishChatConversationUpdatedEvent(ChatConversationUpdatedEvent{
			Conversation:      conversation,
			Members:           members,
			RemovedMessageIDs: result.RemovedMessageIDs,
		}); err != nil {
			log.Printf("chat conversation updated publish failed: %v", err)
		}
	}
}

func announceOnAliceIfRequested(cmd ChatMessageSendCommand, conversation model.ChatConversation, members []model.ChatMember, message model.ChatMessage) model.ChatMessage {
	if !cmd.AnnounceOnAlice || conversation.Type != "direct" || message.Type != "text" {
		return message
	}
	if strings.TrimSpace(message.Text) == "" {
		return message
	}

	recipientEmail := ""
	for _, member := range members {
		if member.Email != cmd.SenderEmail {
			recipientEmail = member.Email
			break
		}
	}
	if recipientEmail == "" {
		log.Printf("chat alice announce skipped: recipient not found for conversation %s", conversation.ID)
		return message
	}

	recipient, err := store.GetUserRepository().FindUserByEmail(recipientEmail)
	if err != nil {
		log.Printf("chat alice announce skipped: %v", err)
		return message
	}
	if recipient.AliceSettings.Disabled {
		log.Printf("chat alice announce skipped: user %s disabled Alice announcements", recipient.Login)
		return message
	}
	if recipient.AliceSettings.AccountID == "" || recipient.AliceSettings.DeviceID == "" {
		log.Printf("chat alice announce skipped: user %s has not configured Alice speaker settings", recipient.Login)
		return message
	}

	client := alice.NewClient()
	if !client.Enabled() {
		log.Printf("chat alice announce skipped: alice service is not configured")
		return message
	}

	if _, err := client.AnnounceScenario(alice.AnnounceRequest{
		AccountID:      recipient.AliceSettings.AccountID,
		DeviceID:       recipient.AliceSettings.DeviceID,
		ScenarioID:     recipient.AliceSettings.ScenarioID,
		InitiatorEmail: cmd.SenderEmail,
		RecipientEmail: recipient.Email,
		ConversationID: conversation.ID,
		MessageID:      message.ID,
		Text:           message.Text,
	}); err != nil {
		log.Printf("chat alice announce failed: %v", err)
		return message
	}

	updated, err := store.GetChatRepository().MarkMessageAliceAnnounced(conversation.ID, message.ID)
	if err != nil {
		log.Printf("chat alice announce mark failed: %v", err)
		return message
	}
	return updated
}

func HandleChatMessageReadCommand(cmd ChatMessageReadCommand) {
	repo := store.GetChatRepository()
	if cmd.MessageID == "" || cmd.ConversationID == "" {
		log.Printf("chat read rejected: conversation_id and message_id are required")
		return
	}
	changed, err := repo.MarkMessagesReadUpToWithResult(cmd.ConversationID, cmd.MessageID, cmd.ReaderEmail, cmd.ReaderLogin)
	if err != nil {
		log.Printf("chat read rejected: %v", err)
		return
	}
	if !changed {
		return
	}

	conversation, members, err := loadChatSnapshot(cmd.ConversationID)
	if err != nil {
		log.Printf("chat read snapshot failed: %v", err)
		return
	}
	messages, err := repo.ListMessages(cmd.ConversationID)
	if err != nil {
		log.Printf("chat read messages failed: %v", err)
		return
	}
	target, affected := findReadTarget(messages, cmd.MessageID)
	if target.ID == "" {
		log.Printf("chat read target not found: %s", cmd.MessageID)
		return
	}
	if err := PublishChatMessageReadUpdatedEvent(ChatMessageReadUpdatedEvent{
		Conversation:       conversation,
		Members:            members,
		MessageID:          cmd.MessageID,
		Message:            target,
		Reader:             ChatParticipant{Email: cmd.ReaderEmail, Login: cmd.ReaderLogin},
		AffectedMessageIDs: affected,
	}); err != nil {
		log.Printf("chat read publish failed: %v", err)
	}
}

func loadChatSnapshot(conversationID string) (model.ChatConversation, []model.ChatMember, error) {
	repo := store.GetChatRepository()
	conversation, err := repo.FindConversationByID(conversationID)
	if err != nil {
		return model.ChatConversation{}, nil, err
	}
	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		return model.ChatConversation{}, nil, err
	}
	return conversation, members, nil
}

func findReadTarget(messages []model.ChatMessage, messageID string) (model.ChatMessage, []string) {
	affected := make([]string, 0)
	for _, message := range messages {
		affected = append(affected, message.ID)
		if message.ID == messageID {
			return message, affected
		}
	}
	return model.ChatMessage{}, nil
}
