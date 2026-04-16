package routes_test

import (
	httpserver "botDashboard/internal/http"
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/go-www/silverlining"
)

var (
	chatHTTPOnce sync.Once
	chatHTTPURL  string
)

func chatHTTPSetup(t *testing.T) {
	t.Helper()

	chatHTTPOnce.Do(func() {
		dir, err := os.MkdirTemp("", "chat-http-test-*")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MIDDLEWARE_OFF=false\n"), 0600); err != nil {
			panic(err)
		}
		if err := os.Chdir(dir); err != nil {
			panic(err)
		}

		if err := os.Setenv("DB_NAME_FILE", filepath.Join(dir, "chat-http.db")); err != nil {
			panic(err)
		}
		if err := os.Setenv("JWT_KEY", "chat-http-test-key"); err != nil {
			panic(err)
		}

		store.InitStore()

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		chatHTTPURL = "http://" + ln.Addr().String()

		srv := &silverlining.Server{Handler: httpserver.HandleRequest}
		go func() {
			_ = srv.Serve(ln)
		}()
	})

	_ = store.GetUserRepository().ClearAll()
	_ = store.GetChatRepository().ClearAll()
}

func createTestUser(t *testing.T, login, email string) model.UserData {
	t.Helper()

	u, err := store.GetUserRepository().CreateUser(login, email, "password")
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u
}

func authToken(t *testing.T, email, login string) string {
	t.Helper()

	token, err := middleware.GetJwt().CreateToken(email, login)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

func doJSONRequest(t *testing.T, method, path, token string, body any) (*nethttp.Response, []byte) {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}

	req, err := nethttp.NewRequest(method, chatHTTPURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, data
}

type chatUserResponse struct {
	Login   string `json:"login"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type chatReceiptResponse struct {
	Login string `json:"login"`
	Email string `json:"email"`
}

type chatMessageResponse struct {
	ID          string                `json:"id"`
	Text        string                `json:"text"`
	SenderLogin string                `json:"sender_login"`
	DeliveredTo []chatReceiptResponse `json:"delivered_to"`
	ReadBy      []chatReceiptResponse `json:"read_by"`
}

type chatMemberResponse struct {
	Login             string `json:"login"`
	Email             string `json:"email"`
	LastReadMessageID string `json:"last_read_message_id"`
}

type chatConversationResponse struct {
	ID             string                `json:"id"`
	CreatedByEmail string                `json:"created_by_email"`
	CreatedByLogin string                `json:"created_by_login"`
	Title          string                `json:"title"`
	UnreadCount    int                   `json:"unread_count"`
	Messages       []chatMessageResponse `json:"messages"`
	Members        []chatMemberResponse  `json:"members"`
}

func TestGetChatUsers(t *testing.T) {
	chatHTTPSetup(t)

	alice := createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	alice.IsAdmin = true
	if err := store.GetUserRepository().UpdateUser(alice, alice.Email); err != nil {
		t.Fatalf("update alice admin flag: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/chat/users", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var users []chatUserResponse
	if err := json.Unmarshal(data, &users); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	logins := []string{users[0].Login, users[1].Login}
	sort.Strings(logins)
	if strings.Join(logins, ",") != "alice,bob" {
		t.Fatalf("unexpected users: %#v", users)
	}
	var foundAdmin bool
	for _, user := range users {
		if user.Email == "alice@example.com" {
			foundAdmin = true
			if !user.IsAdmin {
				t.Fatalf("expected alice to be admin, got %#v", user)
			}
		}
	}
	if !foundAdmin {
		t.Fatalf("alice not found in response: %#v", users)
	}
}

func TestAuthenticatedChatRequestRefreshesSessionToken(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/chat/users", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	refreshedToken := resp.Header.Get("X-Auth-Token")
	if refreshedToken == "" {
		t.Fatal("expected refreshed auth token header")
	}
	if expose := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(expose, "X-Auth-Token") {
		t.Fatalf("expected X-Auth-Token to be exposed for browsers, got %q", expose)
	}

	claims, err := middleware.GetJwt().ValidateToken(refreshedToken)
	if err != nil {
		t.Fatalf("validate refreshed token: %v", err)
	}
	if claims.Email != "alice@example.com" || claims.Login != "alice" {
		t.Fatalf("unexpected refreshed token claims: %#v", claims)
	}
}

func TestPostChatGroupAndGetChatConversations(t *testing.T) {
	chatHTTPSetup(t)

	alice := createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "carol", "carol@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com", "carol@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatConversationResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Title != "Team chat" {
		t.Fatalf("unexpected title: %#v", created)
	}
	if created.CreatedByEmail != alice.Email || created.CreatedByLogin != alice.Login {
		t.Fatalf("expected creator to be current user, got %#v", created)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, "/chat/conversations", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var conversations []chatConversationResponse
	if err := json.Unmarshal(data, &conversations); err != nil {
		t.Fatalf("decode conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected one conversation, got %#v", conversations)
	}
	if conversations[0].Title != "Team chat" {
		t.Fatalf("unexpected conversation payload: %#v", conversations[0])
	}
}

func TestGetChatMessages(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/messages", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID || messages[0].Text != "hello" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestPostChatRead(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err = store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}
	second, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "later")
	if err != nil {
		t.Fatalf("add message 2: %v", err)
	}
	third, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "last")
	if err != nil {
		t.Fatalf("add message 3: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/read", conv.ID), authToken(t, "alice@example.com", "alice"), map[string]string{
		"message_id": second.ID,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	members, err := store.GetChatRepository().ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}

	found := false
	for _, member := range members {
		if member.Email == "alice@example.com" {
			found = true
			if member.LastReadMessageID != second.ID {
				t.Fatalf("expected last read message id %q, got %#v", second.ID, member)
			}
		}
	}
	if !found {
		t.Fatalf("alice member not found: %#v", members)
	}

	messages, err := store.GetChatRepository().ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %#v", messages)
	}
	if len(messages[0].ReadBy) != 1 || messages[0].ReadBy[0].Email != "alice@example.com" {
		t.Fatalf("expected first message to be read by alice, got %#v", messages[0])
	}
	if len(messages[1].ReadBy) != 1 || messages[1].ReadBy[0].Email != "alice@example.com" {
		t.Fatalf("expected second message to be read by alice, got %#v", messages[1])
	}
	if len(messages[2].ReadBy) != 0 {
		t.Fatalf("expected third message to remain unread, got %#v", messages[2])
	}
	if third.ID == "" {
		t.Fatalf("expected third message to exist")
	}
}

func TestPostChatReadRequiresMessageID(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err = store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/read", conv.ID), authToken(t, "alice@example.com", "alice"), map[string]string{})
	if resp.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("expected 400 without message_id, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostChatReadRequiresMembership(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "mallory", "mallory@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/read", conv.ID), authToken(t, "mallory@example.com", "mallory"), map[string]string{
		"message_id": message.ID,
	})
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected 403 for non-member read, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostChatGroupMutationsAllowNonMember(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "carol", "carol@example.com")
	createTestUser(t, "mallory", "mallory@example.com")

	createdResp, createdData := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com"},
	})
	if createdResp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", createdResp.StatusCode, string(createdData))
	}
	var created chatConversationResponse
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatalf("decode created group: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPatch, fmt.Sprintf("/chat/conversations/group/%s", created.ID), authToken(t, "mallory@example.com", "mallory"), map[string]any{
		"title": "hacked",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	renamed, err := store.GetChatRepository().FindConversationByID(created.ID)
	if err != nil {
		t.Fatalf("find conversation after patch: %v", err)
	}
	if renamed.Title != "hacked" {
		t.Fatalf("expected title to update, got %#v", renamed)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/group/%s/members", created.ID), authToken(t, "mallory@example.com", "mallory"), map[string]any{
		"emails": []string{"carol@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 for add members, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodDelete, fmt.Sprintf("/chat/conversations/group/%s/members", created.ID), authToken(t, "mallory@example.com", "mallory"), map[string]any{
		"emails": []string{"bob@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 for delete members, got %d: %s", resp.StatusCode, string(data))
	}

	members, err := store.GetChatRepository().ListConversationMembers(created.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	foundCarol := false
	foundBob := false
	for _, member := range members {
		if member.Email == "carol@example.com" {
			foundCarol = true
		}
		if member.Email == "bob@example.com" {
			foundBob = true
		}
	}
	if !foundCarol {
		t.Fatalf("expected carol to be added, got %#v", members)
	}
	if foundBob {
		t.Fatalf("expected bob to be removed, got %#v", members)
	}
}

func TestDeleteChatGroupMembersAllowsSelfRemovalWithout500(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	createdResp, createdData := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com"},
	})
	if createdResp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", createdResp.StatusCode, string(createdData))
	}
	var created chatConversationResponse
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatalf("decode created group: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodDelete, fmt.Sprintf("/chat/conversations/group/%s/members", created.ID), authToken(t, "bob@example.com", "bob"), map[string]any{
		"emails": []string{"bob@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on self-removal, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestDeleteChatGroupConversation(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	createdResp, createdData := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com"},
	})
	if createdResp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", createdResp.StatusCode, string(createdData))
	}
	var created chatConversationResponse
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatalf("decode created group: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodDelete, fmt.Sprintf("/chat/conversations/group/%s", created.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on group delete, got %d: %s", resp.StatusCode, string(data))
	}

	if _, err := store.GetChatRepository().FindConversationByID(created.ID); err == nil {
		t.Fatal("expected group conversation to be deleted")
	}
}
