package consumer

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"os"
	"path/filepath"
	"testing"
)

type capturingPublisher struct {
	subjects []string
	payloads []any
}

func (p *capturingPublisher) Publish(subject string, payload any) error {
	p.subjects = append(p.subjects, subject)
	p.payloads = append(p.payloads, payload)
	return nil
}

func newChatEventRepo(t *testing.T) *store.ChatRepository {
	t.Helper()

	dir := t.TempDir()
	if err := os.Setenv("DB_NAME_FILE", filepath.Join(dir, "chat-event-test.db")); err != nil {
		t.Fatalf("set DB_NAME_FILE: %v", err)
	}
	store.InitStore()
	repo := store.GetChatRepository()
	t.Cleanup(func() {
		_ = store.GetUserRepository().ClearAll()
		_ = repo.ClearAll()
		producer.ResetPublisherForTest()
	})
	return repo
}

func seedUser(t *testing.T, login, email string) model.UserData {
	t.Helper()
	user, err := store.GetUserRepository().CreateUser(login, email, "password")
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return user
}

func TestHandleChatMessageSendCreatesDirectConversationAndPublishesPersistedEvent(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	seedUser(t, "bob", "bob@example.com")

	pub := &capturingPublisher{}
	producer.SetPublisherForTest(pub)

	HandleChatMessageSend(ChatMessageSendCommand{
		RecipientEmail: "bob@example.com",
		SenderEmail:    "alice@example.com",
		SenderLogin:    "alice",
		Text:           "hello",
	})

	conversations, err := repo.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected one conversation, got %#v", conversations)
	}

	messages, err := repo.ListMessages(conversations[0].ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "hello" {
		t.Fatalf("expected persisted message, got %#v", messages)
	}

	if len(pub.subjects) != 1 || pub.subjects[0] != event.ChatEventMessagePersisted {
		t.Fatalf("expected persisted event publish, got %#v", pub.subjects)
	}

	payload, ok := pub.payloads[0].(event.ChatMessagePersistedEvent)
	if !ok {
		t.Fatalf("unexpected payload type: %#T", pub.payloads[0])
	}
	if payload.Conversation.ID != conversations[0].ID {
		t.Fatalf("expected conversation snapshot in payload, got %#v", payload)
	}
	if len(payload.Members) != 2 {
		t.Fatalf("expected member snapshot in payload, got %#v", payload)
	}
}

func TestHandleChatMessageSendPersistsReplyReference(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	seedUser(t, "bob", "bob@example.com")

	pub := &capturingPublisher{}
	producer.SetPublisherForTest(pub)

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	source, err := repo.AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add source message: %v", err)
	}

	HandleChatMessageSend(ChatMessageSendCommand{
		ConversationID:   conv.ID,
		SenderEmail:      "alice@example.com",
		SenderLogin:      "alice",
		Text:             "reply",
		ReplyToMessageID: source.ID,
	})

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %#v", messages)
	}
	reply := messages[1]
	if reply.ReplyToMessageID != source.ID {
		t.Fatalf("expected reply reference %q, got %#v", source.ID, reply)
	}

	payload, ok := pub.payloads[len(pub.payloads)-1].(event.ChatMessagePersistedEvent)
	if !ok {
		t.Fatalf("unexpected payload type: %#T", pub.payloads[len(pub.payloads)-1])
	}
	if payload.Message.ReplyToMessageID != source.ID {
		t.Fatalf("expected reply reference in persisted event, got %#v", payload.Message)
	}
}

func TestHandleChatMessageReadPublishesUpdatedEventWithoutDuplicateReceipts(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	seedUser(t, "bob", "bob@example.com")

	pub := &capturingPublisher{}
	producer.SetPublisherForTest(pub)

	conv, err := repo.CreateDirectConversation(model.ChatMember{Email: "alice@example.com", Login: "alice"}, model.ChatMember{Email: "bob@example.com", Login: "bob"})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message, err := repo.AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	HandleChatMessageRead(ChatMessageReadCommand{
		ConversationID: conv.ID,
		MessageID:      message.ID,
		ReaderEmail:    "alice@example.com",
		ReaderLogin:    "alice",
	})
	HandleChatMessageRead(ChatMessageReadCommand{
		ConversationID: conv.ID,
		MessageID:      message.ID,
		ReaderEmail:    "alice@example.com",
		ReaderLogin:    "alice",
	})

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %#v", messages)
	}
	if len(messages[0].ReadBy) != 1 {
		t.Fatalf("expected one read receipt, got %#v", messages[0].ReadBy)
	}

	if len(pub.subjects) != 1 {
		t.Fatalf("expected one read-updated publish, got %#v", pub.subjects)
	}
	payload, ok := pub.payloads[len(pub.payloads)-1].(event.ChatMessageReadUpdatedEvent)
	if !ok {
		t.Fatalf("unexpected payload type: %#T", pub.payloads[len(pub.payloads)-1])
	}
	if len(payload.Message.ReadBy) != 1 {
		t.Fatalf("expected read receipts in payload, got %#v", payload)
	}
}

func TestHandleChatMessageSendTrimsOldMessagesAndKeepsPersistedState(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	seedUser(t, "bob", "bob@example.com")

	previousLimit := store.CHAT_MAX_MESSAGES
	store.CHAT_MAX_MESSAGES = 4
	t.Cleanup(func() {
		store.CHAT_MAX_MESSAGES = previousLimit
	})

	pub := &capturingPublisher{}
	producer.SetPublisherForTest(pub)

	HandleChatMessageSend(ChatMessageSendCommand{
		RecipientEmail: "bob@example.com",
		SenderEmail:    "alice@example.com",
		SenderLogin:    "alice",
		Text:           "1",
	})

	conversations, err := repo.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected one conversation, got %#v", conversations)
	}
	convID := conversations[0].ID

	for i := 2; i <= 4; i++ {
		HandleChatMessageSend(ChatMessageSendCommand{
			ConversationID: convID,
			SenderEmail:    "bob@example.com",
			SenderLogin:    "bob",
			Text:           string(rune('0' + i)),
		})
	}

	messages, err := repo.ListMessages(convID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected trimmed message set, got %#v", messages)
	}
	if messages[0].Text != "3" || messages[1].Text != "4" {
		t.Fatalf("expected newest messages to survive trim, got %#v", messages)
	}
	if len(pub.subjects) != 5 {
		t.Fatalf("expected persisted + conversation updated events, got %#v", pub.subjects)
	}
	if pub.subjects[4] != event.ChatEventConversationUpdated {
		t.Fatalf("expected conversation updated event after trim, got %#v", pub.subjects)
	}
	updated, ok := pub.payloads[4].(event.ChatConversationUpdatedEvent)
	if !ok {
		t.Fatalf("unexpected payload type: %#T", pub.payloads[4])
	}
	if len(updated.RemovedMessageIDs) != 2 {
		t.Fatalf("expected removed message ids in conversation update, got %#v", updated)
	}
}
