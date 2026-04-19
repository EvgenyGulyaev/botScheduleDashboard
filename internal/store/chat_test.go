package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newChatRepo(t *testing.T) *ChatRepository {
	t.Helper()

	dir := t.TempDir()
	dbFile := filepath.Join(dir, "chat-test.db")
	if err := os.Setenv("DB_NAME_FILE", dbFile); err != nil {
		t.Fatalf("set DB_NAME_FILE: %v", err)
	}

	InitStore()
	repo := GetChatRepository()
	t.Cleanup(func() {
		_ = repo.ClearAll()
	})
	return repo
}

func TestCreateDirectConversationIsUniqueForPair(t *testing.T) {
	repo := newChatRepo(t)

	first, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}

	second, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	}, model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	})
	if err != nil {
		t.Fatalf("create direct conversation again: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same conversation id, got %q and %q", first.ID, second.ID)
	}

	conversations, err := repo.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}
}

func TestDirectGetOrCreatePreservesExistingMemberState(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}

	message, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	if err := repo.MarkMessageRead(conv.ID, message.ID, "bob@example.com", "bob"); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	before, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members before recreate: %v", err)
	}

	var bobBefore model.ChatMember
	for _, member := range before {
		if member.Email == "bob@example.com" {
			bobBefore = member
		}
	}
	if bobBefore.LastReadMessageID != message.ID {
		t.Fatalf("expected bob to have last read message %q, got %#v", message.ID, bobBefore)
	}

	_, err = repo.CreateDirectConversation(model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	}, model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	})
	if err != nil {
		t.Fatalf("recreate direct conversation: %v", err)
	}

	after, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members after recreate: %v", err)
	}

	var bobAfter model.ChatMember
	for _, member := range after {
		if member.Email == "bob@example.com" {
			bobAfter = member
		}
	}

	if bobAfter.JoinedAt != bobBefore.JoinedAt {
		t.Fatalf("expected joined_at to stay stable, before=%#v after=%#v", bobBefore, bobAfter)
	}
	if bobAfter.LastReadMessageID != bobBefore.LastReadMessageID {
		t.Fatalf("expected last_read_message_id to stay stable, before=%#v after=%#v", bobBefore, bobAfter)
	}
}

func TestCreateConversationPersistsCreatorAndTimestamps(t *testing.T) {
	repo := newChatRepo(t)

	direct, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create direct conversation: %v", err)
	}

	conversations, err := repo.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}

	if direct.CreatedByEmail != "alice@example.com" || direct.CreatedByLogin != "alice" {
		t.Fatalf("expected creator fields to be persisted, got %#v", direct)
	}
	if direct.UpdatedAt.IsZero() || direct.LastMessageID != "" || direct.LastMessageText != "" || !direct.LastMessageAt.IsZero() {
		t.Fatalf("expected fresh conversation metadata to be initialized, got %#v", direct)
	}

	group, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "carol@example.com", Login: "carol"},
		{Email: "dave@example.com", Login: "dave"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	if group.CreatedByEmail != "carol@example.com" || group.CreatedByLogin != "carol" {
		t.Fatalf("expected group creator fields to be persisted, got %#v", group)
	}
	if group.UpdatedAt.IsZero() {
		t.Fatalf("expected group UpdatedAt to be set, got %#v", group)
	}
}

func TestChatModelsRoundTripReplyEditAndPinFields(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	editedAt := now.Add(2 * time.Minute)
	conv.PinnedMessageID = "msg-source"
	conv.PinnedAt = &now
	conv.PinnedByEmail = "alice@example.com"
	conv.PinnedByLogin = "alice"

	err = repo.repo.Update(func(tx *bolt.Tx) error {
		if err := saveConversation(tx, conv); err != nil {
			return err
		}

		return saveMessage(tx, model.ChatMessage{
			ID:               "msg-reply",
			ConversationID:   conv.ID,
			Type:             "text",
			SenderEmail:      "alice@example.com",
			SenderLogin:      "alice",
			Text:             "reply",
			CreatedAt:        now,
			UpdatedAt:        editedAt,
			EditedAt:         &editedAt,
			ReplyToMessageID: "msg-source",
		})
	})
	if err != nil {
		t.Fatalf("seed chat data: %v", err)
	}

	reloadedConversation, err := repo.FindConversationByID(conv.ID)
	if err != nil {
		t.Fatalf("find conversation: %v", err)
	}
	if reloadedConversation.PinnedMessageID != "msg-source" {
		t.Fatalf("expected pinned message id to round-trip, got %#v", reloadedConversation)
	}
	if reloadedConversation.PinnedAt == nil || !reloadedConversation.PinnedAt.Equal(now) {
		t.Fatalf("expected pinned at to round-trip, got %#v", reloadedConversation)
	}
	if reloadedConversation.PinnedByEmail != "alice@example.com" || reloadedConversation.PinnedByLogin != "alice" {
		t.Fatalf("expected pinned by fields to round-trip, got %#v", reloadedConversation)
	}

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].ReplyToMessageID != "msg-source" {
		t.Fatalf("expected reply to id to round-trip, got %#v", messages[0])
	}
	if messages[0].EditedAt == nil || !messages[0].EditedAt.Equal(editedAt) {
		t.Fatalf("expected edited at to round-trip, got %#v", messages[0])
	}
	if !messages[0].UpdatedAt.Equal(editedAt) {
		t.Fatalf("expected updated at to round-trip, got %#v", messages[0])
	}
}

func TestCreateGroupConversationStoresMembersAndUserIndex(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
		{Email: "carol@example.com", Login: "carol"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	userConversations, err := repo.ListUserConversations("alice@example.com")
	if err != nil {
		t.Fatalf("list user conversations: %v", err)
	}
	if len(userConversations) != 1 || userConversations[0] != conv.ID {
		t.Fatalf("expected user conversation index to contain %q, got %#v", conv.ID, userConversations)
	}

	if conv.Title != "Team chat" {
		t.Fatalf("expected title to be persisted, got %q", conv.Title)
	}
}

func TestDeleteGroupConversationRemovesConversationData(t *testing.T) {
	repo := newChatRepo(t)

	group, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}
	if _, err := repo.AddMessage(group.ID, "alice@example.com", "alice", "hello"); err != nil {
		t.Fatalf("add message: %v", err)
	}

	if err := repo.DeleteGroupConversation(group.ID); err != nil {
		t.Fatalf("delete group conversation: %v", err)
	}

	if _, err := repo.FindConversationByID(group.ID); err == nil {
		t.Fatal("expected conversation to be deleted")
	}

	members, err := repo.ListConversationMembers(group.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected members to be deleted, got %#v", members)
	}

	messages, err := repo.ListMessages(group.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected messages to be deleted, got %#v", messages)
	}

	aliceConversations, err := repo.ListUserConversations("alice@example.com")
	if err != nil {
		t.Fatalf("list alice conversations: %v", err)
	}
	if len(aliceConversations) != 0 {
		t.Fatalf("expected alice user index to be cleaned, got %#v", aliceConversations)
	}
}

func TestMarkMessageReadPersistsLastReadMessageID(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	message, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	if err := repo.MarkMessageRead(conv.ID, message.ID, "bob@example.com", "bob"); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	for _, member := range members {
		if member.Email == "bob@example.com" && member.LastReadMessageID != message.ID {
			t.Fatalf("expected bob last read message id to persist, got %#v", member)
		}
	}
}

func TestAddAudioMessageStoresMetadataAndCanBeConsumedOnce(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	result, err := repo.AddAudioMessageWithResult(conv.ID, "alice@example.com", "alice", ChatAudioUpload{
		FilePath:        filepath.Join(t.TempDir(), "voice.webm"),
		MimeType:        "audio/webm",
		SizeBytes:       1234,
		DurationSeconds: 12,
	})
	if err != nil {
		t.Fatalf("add audio message: %v", err)
	}
	if result.Message.Type != "audio" {
		t.Fatalf("expected audio message type, got %#v", result.Message)
	}
	if result.Message.Audio == nil || result.Message.Audio.DurationSeconds != 12 || result.Message.Audio.MimeType != "audio/webm" {
		t.Fatalf("expected audio metadata, got %#v", result.Message.Audio)
	}
	if result.Message.Audio.ExpiresAt.IsZero() {
		t.Fatalf("expected audio expiration to be set, got %#v", result.Message.Audio)
	}

	consumed, err := repo.ConsumeAudioMessage(conv.ID, result.Message.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("consume audio message: %v", err)
	}
	if consumed.Audio == nil || consumed.Audio.ConsumedAt == nil {
		t.Fatalf("expected consumed audio metadata, got %#v", consumed.Audio)
	}
	if consumed.Audio.ConsumedByEmail != "bob@example.com" || consumed.Audio.ConsumedByLogin != "bob" {
		t.Fatalf("expected consumer identity to be stored, got %#v", consumed.Audio)
	}

	if _, err := repo.ConsumeAudioMessage(conv.ID, result.Message.ID, "bob@example.com", "bob"); err == nil {
		t.Fatal("expected second consume to fail")
	}
}

func TestConsumeAudioMessageExpiresStaleAudio(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	audioPath := filepath.Join(t.TempDir(), "expired.webm")
	if err := os.WriteFile(audioPath, []byte("voice"), 0600); err != nil {
		t.Fatalf("write audio file: %v", err)
	}

	result, err := repo.AddAudioMessageWithResult(conv.ID, "alice@example.com", "alice", ChatAudioUpload{
		FilePath:        audioPath,
		MimeType:        "audio/webm",
		SizeBytes:       5,
		DurationSeconds: 12,
	})
	if err != nil {
		t.Fatalf("add audio message: %v", err)
	}

	err = repo.repo.Update(func(tx *bolt.Tx) error {
		message, _, err := loadMessage(tx, conv.ID, result.Message.ID)
		if err != nil {
			return err
		}

		expiredAt := time.Now().UTC().Add(-time.Minute)
		message.Audio.ExpiresAt = expiredAt
		return saveMessage(tx, message)
	})
	if err != nil {
		t.Fatalf("backdate audio expiration: %v", err)
	}

	if _, err := repo.ConsumeAudioMessage(conv.ID, result.Message.ID, "bob@example.com", "bob"); err == nil {
		t.Fatal("expected expired audio consume to fail")
	}

	message, err := repo.FindMessageForMember(conv.ID, result.Message.ID, "bob@example.com")
	if err != nil {
		t.Fatalf("find expired audio message: %v", err)
	}
	if message.Audio == nil || message.Audio.ExpiredAt == nil {
		t.Fatalf("expected audio to be marked expired, got %#v", message.Audio)
	}
	if message.Audio.FilePath != "" {
		t.Fatalf("expected expired audio file path to be cleared, got %#v", message.Audio)
	}
	if _, err := os.Stat(audioPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired audio file to be removed, got err=%v", err)
	}
}

func TestAddImageMessageStoresMetadataAndCanBeConsumedOnce(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	result, err := repo.AddImageMessageWithResult(conv.ID, "alice@example.com", "alice", ChatImageUpload{
		FilePath:  filepath.Join(t.TempDir(), "photo.png"),
		MimeType:  "image/png",
		SizeBytes: 1234,
	})
	if err != nil {
		t.Fatalf("add image message: %v", err)
	}
	if result.Message.Type != "image" {
		t.Fatalf("expected image message type, got %#v", result.Message)
	}
	if result.Message.Image == nil || result.Message.Image.MimeType != "image/png" {
		t.Fatalf("expected image metadata, got %#v", result.Message.Image)
	}
	if result.Message.Image.ExpiresAt.IsZero() {
		t.Fatalf("expected image expiration to be set, got %#v", result.Message.Image)
	}

	consumed, err := repo.ConsumeImageMessage(conv.ID, result.Message.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("consume image message: %v", err)
	}
	if consumed.Image == nil || consumed.Image.ConsumedAt == nil {
		t.Fatalf("expected consumed image metadata, got %#v", consumed.Image)
	}
	if consumed.Image.ConsumedByEmail != "bob@example.com" || consumed.Image.ConsumedByLogin != "bob" {
		t.Fatalf("expected image consumer identity to be stored, got %#v", consumed.Image)
	}

	if _, err := repo.ConsumeImageMessage(conv.ID, result.Message.ID, "bob@example.com", "bob"); err == nil {
		t.Fatal("expected second image consume to fail")
	}
}

func TestConsumeImageMessageExpiresStaleImage(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	imagePath := filepath.Join(t.TempDir(), "expired.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0600); err != nil {
		t.Fatalf("write image file: %v", err)
	}

	result, err := repo.AddImageMessageWithResult(conv.ID, "alice@example.com", "alice", ChatImageUpload{
		FilePath:  imagePath,
		MimeType:  "image/png",
		SizeBytes: 5,
	})
	if err != nil {
		t.Fatalf("add image message: %v", err)
	}

	err = repo.repo.Update(func(tx *bolt.Tx) error {
		message, _, err := loadMessage(tx, conv.ID, result.Message.ID)
		if err != nil {
			return err
		}

		expiredAt := time.Now().UTC().Add(-time.Minute)
		message.Image.ExpiresAt = expiredAt
		return saveMessage(tx, message)
	})
	if err != nil {
		t.Fatalf("backdate image expiration: %v", err)
	}

	if _, err := repo.ConsumeImageMessage(conv.ID, result.Message.ID, "bob@example.com", "bob"); err == nil {
		t.Fatal("expected expired image consume to fail")
	}

	message, err := repo.FindMessageForMember(conv.ID, result.Message.ID, "bob@example.com")
	if err != nil {
		t.Fatalf("find expired image message: %v", err)
	}
	if message.Image == nil || message.Image.ExpiredAt == nil {
		t.Fatalf("expected image to be marked expired, got %#v", message.Image)
	}
	if message.Image.FilePath != "" {
		t.Fatalf("expected expired image file path to be cleared, got %#v", message.Image)
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("expected expired image file to be removed, got err=%v", err)
	}
}

func TestAddMessageTrimsOldestMessagesWhenLimitIsReached(t *testing.T) {
	repo := newChatRepo(t)

	previousLimit := CHAT_MAX_MESSAGES
	CHAT_MAX_MESSAGES = 4
	t.Cleanup(func() {
		CHAT_MAX_MESSAGES = previousLimit
	})

	conv, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	for i := 1; i <= 4; i++ {
		_, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", string(rune('0'+i)))
		if err != nil {
			t.Fatalf("add message %d: %v", i, err)
		}
	}

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after trimming, got %d", len(messages))
	}
	if messages[0].Text != "3" || messages[1].Text != "4" {
		t.Fatalf("expected newest messages to remain, got %#v", messages)
	}
}

func TestTrimMessagesClearsStaleLastReadMessageID(t *testing.T) {
	repo := newChatRepo(t)

	previousLimit := CHAT_MAX_MESSAGES
	CHAT_MAX_MESSAGES = 4
	t.Cleanup(func() {
		CHAT_MAX_MESSAGES = previousLimit
	})

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	_, err = repo.AddMessage(conv.ID, "alice@example.com", "alice", "1")
	if err != nil {
		t.Fatalf("add message 1: %v", err)
	}
	second, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", "2")
	if err != nil {
		t.Fatalf("add message 2: %v", err)
	}

	if err := repo.MarkMessageRead(conv.ID, second.ID, "bob@example.com", "bob"); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	if _, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", "3"); err != nil {
		t.Fatalf("add message 3: %v", err)
	}
	if _, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", "4"); err != nil {
		t.Fatalf("add message 4: %v", err)
	}

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after trimming, got %d", len(messages))
	}
	if messages[0].Text != "3" || messages[1].Text != "4" {
		t.Fatalf("expected surviving messages 3 and 4, got %#v", messages)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, member := range members {
		if member.Email == "bob@example.com" && member.LastReadMessageID != "" {
			t.Fatalf("expected bob last read id to clear after trim, got %#v", member)
		}
	}
}

func TestListMessagesUsesStablePersistedOrdering(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	err = db.GetRepository().Update(func(tx *bolt.Tx) error {
		messageLate := model.ChatMessage{
			ID:             "msg|99999999999999999999|2",
			ConversationID: conv.ID,
			SenderEmail:    "alice@example.com",
			SenderLogin:    "alice",
			Text:           "late",
			CreatedAt:      time.Unix(20, 0).UTC(),
		}
		messageEarly := model.ChatMessage{
			ID:             "msg|00000000000000000001|1",
			ConversationID: conv.ID,
			SenderEmail:    "alice@example.com",
			SenderLogin:    "alice",
			Text:           "early",
			CreatedAt:      time.Unix(10, 0).UTC(),
		}

		if err := saveMessage(tx, messageLate); err != nil {
			return err
		}
		return saveMessage(tx, messageEarly)
	})
	if err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Text != "early" || messages[1].Text != "late" {
		t.Fatalf("expected persisted timestamp ordering, got %#v", messages)
	}
}
