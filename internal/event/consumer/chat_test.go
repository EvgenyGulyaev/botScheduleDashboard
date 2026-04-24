package consumer

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestHandleChatMessageSendPersistsAliceMarkerOnSuccessfulAnnounce(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	recipient := seedUser(t, "bob", "bob@example.com")
	recipient.AliceSettings.AccountID = "home-main"
	recipient.AliceSettings.DeviceID = "speaker-main"
	recipient.AliceSettings.Voice = "oksana"
	if err := store.GetUserRepository().UpdateUser(recipient, recipient.Email); err != nil {
		t.Fatalf("update recipient: %v", err)
	}

	var received struct {
		Voice string `json:"voice"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/announce/scenario" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-1","delivery_id":"delivery-1"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	pub := &capturingPublisher{}
	producer.SetPublisherForTest(pub)

	HandleChatMessageSend(ChatMessageSendCommand{
		RecipientEmail:  "bob@example.com",
		SenderEmail:     "alice@example.com",
		SenderLogin:     "alice",
		Text:            "hello via alice",
		AnnounceOnAlice: true,
	})

	conversations, err := repo.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	messages, err := repo.ListMessages(conversations[0].ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one message, got %#v", messages)
	}
	if !messages[0].AliceAnnounced {
		t.Fatalf("expected message to be marked as announced on Alice, got %#v", messages[0])
	}

	payload, ok := pub.payloads[0].(event.ChatMessagePersistedEvent)
	if !ok {
		t.Fatalf("unexpected payload type: %#T", pub.payloads[0])
	}
	if !payload.Message.AliceAnnounced {
		t.Fatalf("expected persisted event to include Alice marker, got %#v", payload.Message)
	}
	if received.Voice != "oksana" {
		t.Fatalf("expected alice voice to be sent, got %#v", received)
	}
}

func TestHandleChatMessageSendAnnouncesGroupMessageOncePerUniqueAliceDevice(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	bob := seedUser(t, "bob", "bob@example.com")
	carol := seedUser(t, "carol", "carol@example.com")
	dave := seedUser(t, "dave", "dave@example.com")

	bob.AliceSettings.AccountID = "home-main"
	bob.AliceSettings.DeviceID = "speaker-main"
	if err := store.GetUserRepository().UpdateUser(bob, bob.Email); err != nil {
		t.Fatalf("update bob: %v", err)
	}

	carol.AliceSettings.AccountID = "home-main"
	carol.AliceSettings.DeviceID = "speaker-main"
	carol.AliceSettings.Voice = "ermil"
	if err := store.GetUserRepository().UpdateUser(carol, carol.Email); err != nil {
		t.Fatalf("update carol: %v", err)
	}

	dave.AliceSettings.AccountID = "home-main"
	dave.AliceSettings.DeviceID = "speaker-side"
	dave.AliceSettings.Disabled = true
	if err := store.GetUserRepository().UpdateUser(dave, dave.Email); err != nil {
		t.Fatalf("update dave: %v", err)
	}

	var received []struct {
		RecipientEmail string `json:"recipient_email"`
		DeviceID       string `json:"device_id"`
		Voice          string `json:"voice"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/announce/scenario" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			RecipientEmail string `json:"recipient_email"`
			DeviceID       string `json:"device_id"`
			Voice          string `json:"voice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		received = append(received, payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-1","delivery_id":"delivery-1"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	pub := &capturingPublisher{}
	producer.SetPublisherForTest(pub)

	conv, err := repo.CreateGroupConversation("Group", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
		{Email: "carol@example.com", Login: "carol"},
		{Email: "dave@example.com", Login: "dave"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	HandleChatMessageSend(ChatMessageSendCommand{
		ConversationID:  conv.ID,
		SenderEmail:     "alice@example.com",
		SenderLogin:     "alice",
		Text:            "hello group via alice",
		AnnounceOnAlice: true,
	})

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one message, got %#v", messages)
	}
	if !messages[0].AliceAnnounced {
		t.Fatalf("expected message to be marked as announced on Alice, got %#v", messages[0])
	}

	if len(received) != 1 {
		t.Fatalf("expected one unique Alice delivery, got %#v", received)
	}
	if received[0].RecipientEmail != "bob@example.com" || received[0].DeviceID != "speaker-main" {
		t.Fatalf("expected first configured recipient and shared device, got %#v", received[0])
	}
}

func TestAnnounceChatMessageOnAliceUsesVoiceNoticeForAudioMessages(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	bob := seedUser(t, "bob", "bob@example.com")

	bob.AliceSettings.AccountID = "home-main"
	bob.AliceSettings.DeviceID = "speaker-main"
	if err := store.GetUserRepository().UpdateUser(bob, bob.Email); err != nil {
		t.Fatalf("update bob: %v", err)
	}

	var received struct {
		Text           string `json:"text"`
		RecipientEmail string `json:"recipient_email"`
		DeviceID       string `json:"device_id"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/announce/scenario" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-audio","delivery_id":"delivery-audio"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	audioPath := filepath.Join(t.TempDir(), "voice.webm")
	if err := os.WriteFile(audioPath, []byte("voice"), 0600); err != nil {
		t.Fatalf("write audio file: %v", err)
	}

	result, err := repo.AddAudioMessageWithResult(conv.ID, "alice@example.com", "alice", store.ChatAudioUpload{
		FilePath:        audioPath,
		MimeType:        "audio/webm",
		SizeBytes:       5,
		DurationSeconds: 3,
	})
	if err != nil {
		t.Fatalf("add audio message: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}

	updated := event.AnnounceChatMessageOnAlice("alice@example.com", "alice", true, conv, members, result.Message)
	if !updated.AliceAnnounced {
		t.Fatalf("expected audio message to be marked as announced on Alice, got %#v", updated)
	}
	if received.Text != "Передано от alice. Вам пришло голосовое сообщение" {
		t.Fatalf("expected voice notice text, got %#v", received)
	}
	if received.RecipientEmail != "bob@example.com" || received.DeviceID != "speaker-main" {
		t.Fatalf("unexpected Alice target: %#v", received)
	}
}

func TestAnnounceChatMessageOnAliceSkipsRecipientsInQuietHours(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	bob := seedUser(t, "bob", "bob@example.com")

	bob.AliceSettings.AccountID = "home-main"
	bob.AliceSettings.DeviceID = "speaker-main"
	bob.AliceSettings.QuietHoursEnabled = true
	bob.AliceSettings.QuietHoursStart = "00:00"
	bob.AliceSettings.QuietHoursEnd = "23:59"
	if err := store.GetUserRepository().UpdateUser(bob, bob.Email); err != nil {
		t.Fatalf("update bob: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount += 1
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-quiet","delivery_id":"delivery-quiet"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	result, err := repo.AddMessageWithResult(conv.ID, "alice@example.com", "alice", "hello at night", "")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}

	updated := event.AnnounceChatMessageOnAlice("alice@example.com", "alice", true, conv, members, result.Message)
	if updated.AliceAnnounced {
		t.Fatalf("expected message to stay unannounced during quiet hours, got %#v", updated)
	}
	if requestCount != 0 {
		t.Fatalf("expected no Alice requests during quiet hours, got %d", requestCount)
	}
}

func TestAnnounceChatMessageOnAliceSplitsLongTextIntoChunks(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	bob := seedUser(t, "bob", "bob@example.com")

	bob.AliceSettings.AccountID = "home-main"
	bob.AliceSettings.DeviceID = "speaker-main"
	if err := store.GetUserRepository().UpdateUser(bob, bob.Email); err != nil {
		t.Fatalf("update bob: %v", err)
	}

	received := make([]struct {
		Text string `json:"text"`
	}, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		received = append(received, payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-long","delivery_id":"delivery-long"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	longText := "Первый длинный фрагмент сообщения для Алисы, который должен превысить лимит и быть разбит на несколько последовательных частей. " +
		"Второй длинный фрагмент тоже нужен, чтобы проверить, что отправка идёт по кускам, а не одной гигантской строкой."
	result, err := repo.AddMessageWithResult(conv.ID, "alice@example.com", "alice", longText, "")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}

	updated := event.AnnounceChatMessageOnAlice("alice@example.com", "alice", true, conv, members, result.Message)
	if !updated.AliceAnnounced {
		t.Fatalf("expected long message to be announced, got %#v", updated)
	}
	if len(received) < 2 {
		t.Fatalf("expected long message to be split into multiple chunks, got %#v", received)
	}
	for _, payload := range received {
		if !strings.HasPrefix(payload.Text, "Передано от alice. ") {
			t.Fatalf("expected sender prefix in every chunk, got %#v", received)
		}
	}
}

func TestAnnounceChatMessageOnAliceAddsReplyContext(t *testing.T) {
	repo := newChatEventRepo(t)
	seedUser(t, "alice", "alice@example.com")
	bob := seedUser(t, "bob", "bob@example.com")

	bob.AliceSettings.AccountID = "home-main"
	bob.AliceSettings.DeviceID = "speaker-main"
	if err := store.GetUserRepository().UpdateUser(bob, bob.Email); err != nil {
		t.Fatalf("update bob: %v", err)
	}

	var received struct {
		Text string `json:"text"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-reply","delivery_id":"delivery-reply"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	source, err := repo.AddMessage(conv.ID, "bob@example.com", "nika", "Исходный текст сообщения")
	if err != nil {
		t.Fatalf("add source message: %v", err)
	}
	replyResult, err := repo.AddMessageWithResult(conv.ID, "alice@example.com", "alice", "Ответный текст", source.ID)
	if err != nil {
		t.Fatalf("add reply message: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}

	updated := event.AnnounceChatMessageOnAlice("alice@example.com", "alice", true, conv, members, replyResult.Message)
	if !updated.AliceAnnounced {
		t.Fatalf("expected reply message to be announced, got %#v", updated)
	}
	expected := "Передано от alice. В ответ на сообщение \"Исходный текст сообщения\" от пользователя nika. Ответный текст"
	if received.Text != expected {
		t.Fatalf("expected reply context in Alice text, got %#v", received)
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
