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
