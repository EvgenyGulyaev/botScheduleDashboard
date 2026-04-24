package event

import (
	"botDashboard/internal/alice"
	"botDashboard/internal/model"
	"botDashboard/internal/push"
	"botDashboard/internal/store"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const aliceAnnouncementChunkLimit = 220

var moscowLocation = loadMoscowLocation()

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
	result.Message = AnnounceChatMessageOnAlice(cmd.SenderEmail, cmd.SenderLogin, cmd.AnnounceOnAlice, conversation, members, result.Message)
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

func AnnounceChatMessageOnAlice(senderEmail, senderLogin string, enabled bool, conversation model.ChatConversation, members []model.ChatMember, message model.ChatMessage) model.ChatMessage {
	updated, _ := AnnounceChatMessageOnAliceWithCount(senderEmail, senderLogin, enabled, conversation, members, message)
	return updated
}

func AnnounceChatMessageOnAliceWithCount(senderEmail, senderLogin string, enabled bool, conversation model.ChatConversation, members []model.ChatMember, message model.ChatMessage) (model.ChatMessage, int) {
	if !enabled {
		return message, 0
	}

	announcementChunks := buildAliceAnnouncementChunks(senderLogin, message)
	if len(announcementChunks) == 0 {
		return message, 0
	}

	client := alice.NewClient()
	if !client.Enabled() {
		log.Printf("chat alice announce skipped: alice service is not configured")
		return message, 0
	}

	recipients := collectAliceRecipients(senderEmail, conversation, members)
	if len(recipients) == 0 {
		return message, 0
	}

	deliveries := 0
	for _, recipient := range recipients {
		recipientDelivered := true
		for _, announcementText := range announcementChunks {
			if _, err := client.AnnounceScenario(alice.AnnounceRequest{
				AccountID:      recipient.AliceSettings.AccountID,
				HouseholdID:    recipient.AliceSettings.HouseholdID,
				RoomID:         recipient.AliceSettings.RoomID,
				DeviceID:       recipient.AliceSettings.DeviceID,
				ScenarioID:     recipient.AliceSettings.ScenarioID,
				Voice:          recipient.AliceSettings.Voice,
				InitiatorEmail: senderEmail,
				RecipientEmail: recipient.Email,
				ConversationID: conversation.ID,
				MessageID:      message.ID,
				Text:           announcementText,
			}); err != nil {
				log.Printf("chat alice announce failed for %s: %v", recipient.Email, err)
				recipientDelivered = false
				break
			}
		}

		if recipientDelivered {
			deliveries += 1
		}
	}
	if deliveries == 0 {
		return message, 0
	}

	if message.ID == "" || conversation.ID == "" {
		message.AliceAnnounced = true
		return message, deliveries
	}

	updated, err := store.GetChatRepository().MarkMessageAliceAnnounced(conversation.ID, message.ID)
	if err != nil {
		log.Printf("chat alice announce mark failed: %v", err)
		return message, deliveries
	}
	return updated, deliveries
}

func buildAliceAnnouncementChunks(senderLogin string, message model.ChatMessage) []string {
	prefix := buildAliceAnnouncementPrefix(senderLogin)
	switch message.Type {
	case "text":
		return splitAliceAnnouncementTextWithPrefix(strings.TrimSpace(message.Text), prefix, aliceAnnouncementChunkLimit)
	case "audio":
		if message.Audio == nil {
			return nil
		}
		return []string{prefix + "Вам пришло голосовое сообщение"}
	default:
		return nil
	}
}

func buildAliceAnnouncementPrefix(senderLogin string) string {
	senderLogin = strings.TrimSpace(senderLogin)
	if senderLogin == "" {
		return ""
	}

	return "Передано от " + senderLogin + ". "
}

func collectAliceRecipients(senderEmail string, conversation model.ChatConversation, members []model.ChatMember) []model.UserData {
	if conversation.Type != "direct" && conversation.Type != "group" {
		return nil
	}

	recipients := make([]model.UserData, 0)
	seenDevices := make(map[string]struct{})
	nowMoscow := time.Now().In(moscowLocation)
	for _, member := range members {
		if member.Email == "" || member.Email == senderEmail {
			continue
		}

		recipient, err := store.GetUserRepository().FindUserByEmail(member.Email)
		if err != nil {
			log.Printf("chat alice announce skipped: %v", err)
			continue
		}
		if recipient.AliceSettings.Disabled {
			log.Printf("chat alice announce skipped: user %s disabled Alice announcements", recipient.Login)
			continue
		}
		if recipient.AliceSettings.AccountID == "" || recipient.AliceSettings.DeviceID == "" {
			log.Printf("chat alice announce skipped: user %s has not configured Alice speaker settings", recipient.Login)
			continue
		}
		if isAliceQuietHoursActive(recipient.AliceSettings, nowMoscow) {
			log.Printf("chat alice announce skipped: user %s is in quiet hours", recipient.Login)
			continue
		}

		deviceKey := recipient.AliceSettings.AccountID + "|" + recipient.AliceSettings.DeviceID
		if _, exists := seenDevices[deviceKey]; exists {
			continue
		}

		seenDevices[deviceKey] = struct{}{}
		recipients = append(recipients, recipient)
	}

	return recipients
}

func splitAliceAnnouncementTextWithPrefix(text, prefix string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if prefix == "" {
		return splitAliceAnnouncementText(text, limit)
	}

	chunks := splitAliceAnnouncementText(text, effectiveAliceAnnouncementChunkLimit(prefix, limit))
	if len(chunks) == 0 {
		return nil
	}

	prefixed := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		prefixed = append(prefixed, prefix+chunk)
	}
	return prefixed
}

func splitAliceAnnouncementText(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	chunks := make([]string, 0)
	current := ""
	for _, word := range words {
		if utf8.RuneCountInString(word) > limit {
			if current != "" {
				chunks = append(chunks, current)
				current = ""
			}
			chunks = append(chunks, splitLongAliceWord(word, limit)...)
			continue
		}

		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if utf8.RuneCountInString(candidate) > limit {
			chunks = append(chunks, current)
			current = word
			continue
		}
		current = candidate
	}
	if current != "" {
		chunks = append(chunks, current)
	}

	if len(chunks) <= 1 {
		return chunks
	}

	labelled := make([]string, 0, len(chunks))
	for index, chunk := range chunks {
		labelled = append(labelled, "Часть "+itoa(index+1)+" из "+itoa(len(chunks))+". "+chunk)
	}
	return labelled
}

func effectiveAliceAnnouncementChunkLimit(prefix string, limit int) int {
	if limit <= 0 {
		return limit
	}

	effective := limit - utf8.RuneCountInString(prefix)
	if effective < 40 {
		return 40
	}

	return effective
}

func splitLongAliceWord(word string, limit int) []string {
	runes := []rune(word)
	if len(runes) == 0 {
		return nil
	}

	chunks := make([]string, 0, (len(runes)/limit)+1)
	for start := 0; start < len(runes); start += limit {
		end := start + limit
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func isAliceQuietHoursActive(settings model.UserAliceSettings, now time.Time) bool {
	if !settings.QuietHoursEnabled {
		return false
	}

	start, err := time.Parse("15:04", settings.QuietHoursStart)
	if err != nil {
		return false
	}
	end, err := time.Parse("15:04", settings.QuietHoursEnd)
	if err != nil {
		return false
	}

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := start.Hour()*60 + start.Minute()
	endMinutes := end.Hour()*60 + end.Minute()

	if startMinutes == endMinutes {
		return true
	}
	if startMinutes < endMinutes {
		return currentMinutes >= startMinutes && currentMinutes < endMinutes
	}

	return currentMinutes >= startMinutes || currentMinutes < endMinutes
}

func loadMoscowLocation() *time.Location {
	location, err := time.LoadLocation("Europe/Moscow")
	if err == nil {
		return location
	}

	return time.FixedZone("Europe/Moscow", 3*60*60)
}

func itoa(value int) string {
	return strconv.Itoa(value)
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
