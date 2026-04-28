package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestPresencePersistsLastActiveAndLastSeen(t *testing.T) {
	repo := newChatRepo(t)

	active, transitionedOnline, err := repo.MarkUserOnline("alice@example.com", "alice")
	if err != nil {
		t.Fatalf("mark online: %v", err)
	}
	if !transitionedOnline {
		t.Fatal("expected first online mark to transition online")
	}
	if active.LastActiveAt.IsZero() {
		t.Fatalf("expected last_active_at to be stored, got %#v", active)
	}
	if !repo.IsUserOnline("alice@example.com") {
		t.Fatal("expected alice to be online")
	}

	seen, transitionedOffline, err := repo.MarkUserOffline("alice@example.com", "alice")
	if err != nil {
		t.Fatalf("mark offline: %v", err)
	}
	if !transitionedOffline {
		t.Fatal("expected final offline mark to transition offline")
	}
	if seen.LastSeenAt.IsZero() {
		t.Fatalf("expected last_seen_at to be stored, got %#v", seen)
	}
	if seen.LastSeenAt.Before(active.LastActiveAt) {
		t.Fatalf("expected last_seen_at to be at or after last_active_at, got %#v then %#v", active, seen)
	}
	if repo.IsUserOnline("alice@example.com") {
		t.Fatal("expected alice to be offline")
	}
}

func TestPresenceExpiresWhenHeartbeatIsStale(t *testing.T) {
	repo := newChatRepo(t)

	previousTTL := CHAT_PRESENCE_ONLINE_TTL
	CHAT_PRESENCE_ONLINE_TTL = 100 * time.Millisecond
	t.Cleanup(func() {
		CHAT_PRESENCE_ONLINE_TTL = previousTTL
	})

	if _, _, err := repo.MarkUserOnline("alice@example.com", "alice"); err != nil {
		t.Fatalf("mark online: %v", err)
	}
	presence, err := repo.UserPresence("alice@example.com")
	if err != nil {
		t.Fatalf("presence after online: %v", err)
	}
	if !presence.Online {
		t.Fatalf("expected fresh heartbeat to be online, got %#v", presence)
	}

	time.Sleep(150 * time.Millisecond)
	presence, err = repo.UserPresence("alice@example.com")
	if err != nil {
		t.Fatalf("presence after ttl: %v", err)
	}
	if presence.Online {
		t.Fatalf("expected stale heartbeat to expire, got %#v", presence)
	}
}

func TestPresenceHeartbeatReactivatesStaleUser(t *testing.T) {
	repo := newChatRepo(t)

	previousTTL := CHAT_PRESENCE_ONLINE_TTL
	CHAT_PRESENCE_ONLINE_TTL = 100 * time.Millisecond
	t.Cleanup(func() {
		CHAT_PRESENCE_ONLINE_TTL = previousTTL
	})

	if _, _, err := repo.MarkUserOnline("alice@example.com", "alice"); err != nil {
		t.Fatalf("mark online: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	presence, transitioned, err := repo.MarkUserOnline("alice@example.com", "alice")
	if err != nil {
		t.Fatalf("heartbeat online: %v", err)
	}
	if !transitioned {
		t.Fatal("expected stale user to transition online after heartbeat")
	}
	if !presence.Online {
		t.Fatalf("expected heartbeat to reactivate user, got %#v", presence)
	}
}

func TestDraftSaveFetchAndEmptyTextClears(t *testing.T) {
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

	saved, err := repo.SaveChatDraft(conv.ID, "alice@example.com", "remember this")
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}
	if saved.Text != "remember this" || saved.ConversationID != conv.ID || saved.UserEmail != "alice@example.com" {
		t.Fatalf("unexpected saved draft: %#v", saved)
	}
	if saved.UpdatedAt.IsZero() {
		t.Fatalf("expected draft updated_at to be set, got %#v", saved)
	}

	fetched, ok, err := repo.GetChatDraft(conv.ID, "alice@example.com")
	if err != nil {
		t.Fatalf("fetch draft: %v", err)
	}
	if !ok || fetched.Text != "remember this" || !fetched.UpdatedAt.Equal(saved.UpdatedAt) {
		t.Fatalf("unexpected fetched draft: ok=%v draft=%#v saved=%#v", ok, fetched, saved)
	}

	if _, err := repo.SaveChatDraft(conv.ID, "alice@example.com", ""); err != nil {
		t.Fatalf("clear draft with empty text: %v", err)
	}
	fetched, ok, err = repo.GetChatDraft(conv.ID, "alice@example.com")
	if err != nil {
		t.Fatalf("fetch cleared draft: %v", err)
	}
	if ok {
		t.Fatalf("expected draft to be cleared, got %#v", fetched)
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

func TestCreateGroupConversationAssignsOwnerAndMemberRoles(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	roles := map[string]string{}
	for _, member := range members {
		roles[member.Email] = member.Role
	}
	if roles["alice@example.com"] != "owner" {
		t.Fatalf("expected creator to be owner, got roles %#v", roles)
	}
	if roles["bob@example.com"] != "member" {
		t.Fatalf("expected invited user to be member, got roles %#v", roles)
	}
}

func TestGroupMemberRoleUpdateAndLegacyRoleFallback(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	if _, err := repo.SetGroupMemberRole(conv.ID, "bob@example.com", "admin"); err != nil {
		t.Fatalf("set group member role: %v", err)
	}

	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	roles := map[string]string{}
	for _, member := range members {
		roles[member.Email] = member.Role
	}
	if roles["bob@example.com"] != "admin" {
		t.Fatalf("expected bob to be admin, got roles %#v", roles)
	}

	err = repo.repo.Update(func(tx *bolt.Tx) error {
		for _, member := range members {
			member.Role = ""
			if err := saveMember(tx, member); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("clear stored roles: %v", err)
	}

	members, err = repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list legacy members: %v", err)
	}
	roles = map[string]string{}
	for _, member := range members {
		roles[member.Email] = member.Role
	}
	if roles["alice@example.com"] != "owner" || roles["bob@example.com"] != "member" {
		t.Fatalf("expected legacy fallback owner/member roles, got %#v", roles)
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

func TestReadPointAdvancesOnlyAfterExplicitMarkRead(t *testing.T) {
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

	message, err := repo.AddMessage(conv.ID, "bob@example.com", "bob", "unread")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	if _, err := repo.ListMessages(conv.ID); err != nil {
		t.Fatalf("list messages: %v", err)
	}
	members, err := repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, member := range members {
		if member.Email == "alice@example.com" && member.LastReadMessageID != "" {
			t.Fatalf("expected list to leave read point empty, got %#v", member)
		}
	}

	if err := repo.MarkMessagesReadUpTo(conv.ID, message.ID, "alice@example.com", "alice"); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	members, err = repo.ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members after read: %v", err)
	}
	for _, member := range members {
		if member.Email == "alice@example.com" && member.LastReadMessageID != message.ID {
			t.Fatalf("expected explicit read to advance read point to %q, got %#v", message.ID, member)
		}
	}
}

func TestClientMessageIDDedupeReturnsExistingPersistedMessage(t *testing.T) {
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

	first, err := repo.AddMessageWithClientMessageID(conv.ID, "alice@example.com", "alice", "hello", "client-1", "")
	if err != nil {
		t.Fatalf("add first message: %v", err)
	}
	second, err := repo.AddMessageWithClientMessageID(conv.ID, "alice@example.com", "alice", "hello again", "client-1", "")
	if err != nil {
		t.Fatalf("add duplicate message: %v", err)
	}

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one persisted message after duplicate client id, got %#v", messages)
	}
	if first.ID != second.ID || second.Text != "hello" || second.ClientMessageID != "client-1" {
		t.Fatalf("expected duplicate send to return original message, first=%#v second=%#v", first, second)
	}
}

func TestFavoriteMessagesArePrivatePerUser(t *testing.T) {
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
	message, err := repo.AddMessage(conv.ID, "alice@example.com", "alice", "favorite me")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	favorited, err := repo.SetMessageFavorite(conv.ID, message.ID, "bob@example.com")
	if err != nil {
		t.Fatalf("favorite message: %v", err)
	}
	if !favorited.Favorite {
		t.Fatalf("expected favorite flag on returned message, got %#v", favorited)
	}

	aliceMessages, err := repo.HydrateMessageFavorites([]model.ChatMessage{message}, "alice@example.com")
	if err != nil {
		t.Fatalf("hydrate alice favorites: %v", err)
	}
	if aliceMessages[0].Favorite {
		t.Fatalf("expected alice not to see bob favorite, got %#v", aliceMessages[0])
	}

	bobMessages, err := repo.HydrateMessageFavorites([]model.ChatMessage{message}, "bob@example.com")
	if err != nil {
		t.Fatalf("hydrate bob favorites: %v", err)
	}
	if !bobMessages[0].Favorite {
		t.Fatalf("expected bob favorite flag, got %#v", bobMessages[0])
	}

	favorites, err := repo.ListFavoriteMessages("bob@example.com")
	if err != nil {
		t.Fatalf("list bob favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != message.ID || !favorites[0].Favorite {
		t.Fatalf("unexpected bob favorites: %#v", favorites)
	}

	if _, err := repo.DeleteMessageFavorite(conv.ID, message.ID, "bob@example.com"); err != nil {
		t.Fatalf("delete favorite: %v", err)
	}
	favorites, err = repo.ListFavoriteMessages("bob@example.com")
	if err != nil {
		t.Fatalf("list bob favorites after delete: %v", err)
	}
	if len(favorites) != 0 {
		t.Fatalf("expected bob favorites to be empty, got %#v", favorites)
	}
}

func TestForwardMessagesRequiresSourceAndTargetMembership(t *testing.T) {
	repo := newChatRepo(t)

	source, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "carol@example.com",
		Login: "carol",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	message, err := repo.AddMessage(source.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add source message: %v", err)
	}

	if _, err := repo.ForwardMessages(target.ID, source.ID, []string{message.ID}, "bob@example.com", "bob"); err == nil {
		t.Fatal("expected forwarding to fail when actor is not target member")
	}
	if _, err := repo.ForwardMessages(target.ID, source.ID, []string{message.ID}, "carol@example.com", "carol"); err == nil {
		t.Fatal("expected forwarding to fail when actor is not source member")
	}
}

func TestForwardMessagesCopiesTextAndSafeMediaNoticesWithMetadata(t *testing.T) {
	repo := newChatRepo(t)

	source, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "carol@example.com",
		Login: "carol",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	textMessage, err := repo.AddMessage(source.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add text source message: %v", err)
	}
	audioResult, err := repo.AddAudioMessageWithResult(source.ID, "bob@example.com", "bob", ChatAudioUpload{
		FilePath:        filepath.Join(t.TempDir(), "voice.webm"),
		MimeType:        "audio/webm",
		SizeBytes:       10,
		DurationSeconds: 3,
	})
	if err != nil {
		t.Fatalf("add audio source message: %v", err)
	}

	result, err := repo.ForwardMessages(target.ID, source.ID, []string{textMessage.ID, audioResult.Message.ID}, "alice@example.com", "alice")
	if err != nil {
		t.Fatalf("forward messages: %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 forwarded messages, got %#v", result.Messages)
	}
	if result.Messages[0].Type != "text" || result.Messages[0].Text != "hello" {
		t.Fatalf("expected text message copy, got %#v", result.Messages[0])
	}
	if result.Messages[1].Type != "text" || result.Messages[1].Audio != nil || !strings.Contains(result.Messages[1].Text, "audio") {
		t.Fatalf("expected safe audio notice, got %#v", result.Messages[1])
	}
	for _, forwarded := range result.Messages {
		if forwarded.SenderEmail != "alice@example.com" || forwarded.ConversationID != target.ID {
			t.Fatalf("unexpected forwarding sender/target: %#v", forwarded)
		}
		if forwarded.ForwardedFrom == nil || forwarded.ForwardedFrom.OriginalConversationID != source.ID || forwarded.ForwardedFrom.OriginalSenderEmail != "bob@example.com" {
			t.Fatalf("expected forwarded metadata, got %#v", forwarded)
		}
	}
	if result.Messages[0].ForwardedFrom.OriginalMessageID != textMessage.ID {
		t.Fatalf("expected original text id metadata, got %#v", result.Messages[0].ForwardedFrom)
	}
}

func TestForwardMessagesOnlyReturnsMessagesThatSurviveTrimming(t *testing.T) {
	repo := newChatRepo(t)

	source, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := repo.CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "carol@example.com",
		Login: "carol",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	sourceIDs := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		message, err := repo.AddMessage(source.ID, "bob@example.com", "bob", fmt.Sprintf("source %d", i))
		if err != nil {
			t.Fatalf("add source message %d: %v", i, err)
		}
		sourceIDs = append(sourceIDs, message.ID)
	}

	previousLimit := CHAT_MAX_MESSAGES
	CHAT_MAX_MESSAGES = 4
	t.Cleanup(func() {
		CHAT_MAX_MESSAGES = previousLimit
	})

	for i := 1; i <= 3; i++ {
		if _, err := repo.AddMessage(target.ID, "carol@example.com", "carol", fmt.Sprintf("target %d", i)); err != nil {
			t.Fatalf("add target message %d: %v", i, err)
		}
	}

	result, err := repo.ForwardMessages(target.ID, source.ID, sourceIDs, "alice@example.com", "alice")
	if err != nil {
		t.Fatalf("forward messages: %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("expected only surviving forwarded messages, got %#v", result.Messages)
	}
	if result.Messages[0].Text != "source 4" || result.Messages[1].Text != "source 5" {
		t.Fatalf("expected newest forwarded messages to survive, got %#v", result.Messages)
	}

	messages, err := repo.ListMessages(target.ID)
	if err != nil {
		t.Fatalf("list target messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected target history under limit after forwarding, got %#v", messages)
	}
	if messages[0].ID != result.Messages[0].ID || messages[1].ID != result.Messages[1].ID {
		t.Fatalf("expected returned messages to match stored messages, result=%#v stored=%#v", result.Messages, messages)
	}
}

func TestDeliveredLifecycleIsIdempotentAndMonotonic(t *testing.T) {
	repo := newChatRepo(t)

	conv, err := repo.CreateGroupConversation("Team chat", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
		{Email: "carol@example.com", Login: "carol"},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	message, err := repo.AddMessageWithClientMessageID(conv.ID, "alice@example.com", "alice", "hello", "client-1", "")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}
	if message.DeliveryStatus != "sent" || message.DeliveredToCount != 0 || message.ReadByCount != 0 {
		t.Fatalf("expected new message to be sent only, got %#v", message)
	}

	delivered, changed, err := repo.MarkMessageDelivered(conv.ID, message.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if !changed || delivered.DeliveryStatus != "delivered" || delivered.DeliveredToCount != 1 {
		t.Fatalf("expected delivered transition, changed=%v message=%#v", changed, delivered)
	}

	delivered, changed, err = repo.MarkMessageDelivered(conv.ID, message.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("mark duplicate delivered: %v", err)
	}
	if changed || delivered.DeliveredToCount != 1 || delivered.DeliveryStatus != "delivered" {
		t.Fatalf("expected duplicate delivery ack to be idempotent, changed=%v message=%#v", changed, delivered)
	}

	readChanged, err := repo.MarkMessagesReadUpToWithResult(conv.ID, message.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if !readChanged {
		t.Fatal("expected first read update to change state")
	}
	readChanged, err = repo.MarkMessagesReadUpToWithResult(conv.ID, message.ID, "bob@example.com", "bob")
	if err != nil {
		t.Fatalf("mark duplicate read: %v", err)
	}
	if readChanged {
		t.Fatal("expected duplicate read update to be idempotent")
	}

	messages, err := repo.ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].DeliveryStatus != "read" || messages[0].ReadByCount != 1 || messages[0].DeliveredToCount != 1 {
		t.Fatalf("expected read status to be monotonic over delivered, got %#v", messages)
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
