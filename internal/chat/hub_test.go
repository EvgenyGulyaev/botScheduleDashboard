package chat

import (
	"botDashboard/internal/event"
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"context"
	"encoding/json"
	"io"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type testCommandPublisher struct {
	mu       sync.Mutex
	sendCmds []event.ChatMessageSendCommand
	readCmds []event.ChatMessageReadCommand
}

func (p *testCommandPublisher) PublishChatMessageSendCommand(cmd event.ChatMessageSendCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendCmds = append(p.sendCmds, cmd)
	return nil
}

func (p *testCommandPublisher) PublishChatMessageReadCommand(cmd event.ChatMessageReadCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readCmds = append(p.readCmds, cmd)
	return nil
}

var chatGatewayOnce sync.Once

func newChatGatewayTestRepo(t *testing.T) *store.ChatRepository {
	t.Helper()

	chatGatewayOnce.Do(func() {
		dir, err := os.MkdirTemp("", "chat-gateway-test-*")
		if err != nil {
			panic(err)
		}
		if err := os.Setenv("DB_NAME_FILE", filepath.Join(dir, "chat-gateway-test.db")); err != nil {
			panic(err)
		}
		store.InitStore()
	})
	t.Cleanup(func() {
		_ = store.GetChatRepository().ClearAll()
		_ = store.GetUserRepository().ClearAll()
	})
	return store.GetChatRepository()
}

func createGatewayUser(t *testing.T, login, email string) model.UserData {
	t.Helper()
	user, err := store.GetUserRepository().CreateUser(login, email, "password")
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return user
}

func authToken(t *testing.T, email, login string) string {
	t.Helper()
	token, err := middleware.GetJwt().CreateToken(email, login)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

func newChatGatewayServer(t *testing.T) (*Server, *testCommandPublisher) {
	t.Helper()
	pub := &testCommandPublisher{}
	return NewServer(WithCommandPublisher(pub), WithNATSDisabledForTest()), pub
}

func wsClientURL(base string) string {
	return "ws" + base[len("http"):] + "/chat/ws"
}

func dialChatWS(t *testing.T, baseURL, token string) net.Conn {
	t.Helper()

	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP(nethttp.Header{
			"Authorization": []string{"Bearer " + token},
		}),
	}
	conn, _, _, err := dialer.Dial(context.Background(), wsClientURL(baseURL))
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return conn
}

func dialChatWSWithQueryToken(t *testing.T, baseURL, token string) net.Conn {
	t.Helper()

	conn, _, _, err := ws.Dial(context.Background(), wsClientURL(baseURL)+"?token="+token)
	if err != nil {
		t.Fatalf("dial websocket with query token: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return conn
}

func readGatewayEnvelope(t *testing.T, conn net.Conn) gatewayEnvelope {
	t.Helper()

	msgs, err := wsutil.ReadServerMessage(conn, nil)
	if err != nil {
		t.Fatalf("read server message: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected websocket message")
	}
	var env gatewayEnvelope
	if err := json.Unmarshal(msgs[len(msgs)-1].Payload, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

func waitForCount(t *testing.T, hub *Hub, email string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.ClientCount(email) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected client count %d for %s, got %d", want, email, hub.ClientCount(email))
}

func TestServerRegistersAuthenticatedClient(t *testing.T) {
	newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "alice", "alice@example.com")

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWS(t, ts.URL, authToken(t, user.Email, user.Login))
	waitForCount(t, srv.Hub, user.Email, 1)

	_ = conn.Close()
	waitForCount(t, srv.Hub, user.Email, 0)
}

func TestServerRegistersClientWithQueryToken(t *testing.T) {
	newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "alice", "alice@example.com")

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWSWithQueryToken(t, ts.URL, authToken(t, user.Email, user.Login))
	waitForCount(t, srv.Hub, user.Email, 1)

	_ = conn.Close()
	waitForCount(t, srv.Hub, user.Email, 0)
}

func TestHubRoutesPersistedEventToConnectedParticipants(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")

	conv, err := repo.CreateDirectConversation(model.ChatMember{Email: alice.Email, Login: alice.Login}, model.ChatMember{Email: bob.Email, Login: bob.Login})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	msg, err := repo.AddMessage(conv.ID, alice.Email, alice.Login, "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	aliceConn := dialChatWS(t, ts.URL, authToken(t, alice.Email, alice.Login))
	bobConn := dialChatWS(t, ts.URL, authToken(t, bob.Email, bob.Login))
	waitForCount(t, srv.Hub, alice.Email, 1)
	waitForCount(t, srv.Hub, bob.Email, 1)

	srv.Hub.HandleChatMessagePersisted(event.ChatMessagePersistedEvent{
		Conversation: conv,
		Members: []model.ChatMember{
			{ConversationID: conv.ID, Email: alice.Email, Login: alice.Login},
			{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login},
		},
		Message: msg,
	})

	aliceEnv := readGatewayEnvelope(t, aliceConn)
	if aliceEnv.Event != GatewayEventMessagePersisted {
		t.Fatalf("unexpected alice event: %#v", aliceEnv)
	}
	bobEnv := readGatewayEnvelope(t, bobConn)
	if bobEnv.Event != GatewayEventMessagePersisted {
		t.Fatalf("unexpected bob event: %#v", bobEnv)
	}
}

func TestHubRoutesReadAndConversationUpdateEvents(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")

	conv, err := repo.CreateGroupConversation("Team", []model.ChatMember{
		{Email: alice.Email, Login: alice.Login},
		{Email: bob.Email, Login: bob.Login},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	msg, err := repo.AddMessage(conv.ID, alice.Email, alice.Login, "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	aliceConn := dialChatWS(t, ts.URL, authToken(t, alice.Email, alice.Login))
	waitForCount(t, srv.Hub, alice.Email, 1)

	srv.Hub.HandleChatMessageReadUpdated(event.ChatMessageReadUpdatedEvent{
		Conversation: conv,
		Members: []model.ChatMember{
			{ConversationID: conv.ID, Email: alice.Email, Login: alice.Login},
			{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login},
		},
		MessageID: msg.ID,
		Message:   msg,
		Reader:    event.ChatParticipant{Email: bob.Email, Login: bob.Login},
	})
	srv.Hub.HandleChatConversationUpdated(event.ChatConversationUpdatedEvent{
		Conversation:      conv,
		Members:           []model.ChatMember{{ConversationID: conv.ID, Email: alice.Email, Login: alice.Login}},
		RemovedMessageIDs: []string{"m-1", "m-2"},
	})

	first := readGatewayEnvelope(t, aliceConn)
	if first.Event != GatewayEventMessageReadUpdated {
		t.Fatalf("unexpected first event: %#v", first)
	}
	second := readGatewayEnvelope(t, aliceConn)
	if second.Event != GatewayEventConversationUpdated {
		t.Fatalf("unexpected second event: %#v", second)
	}
}

func TestServerRejectsMissingAuthorization(t *testing.T) {
	newChatGatewayTestRepo(t)
	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	req, err := nethttp.NewRequest(nethttp.MethodGet, ts.URL+"/chat/ws", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestServerRegistersAuthenticatedClientWithoutUserRepositoryLookup(t *testing.T) {
	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWS(t, ts.URL, authToken(t, "ghost@example.com", "ghost"))
	waitForCount(t, srv.Hub, "ghost@example.com", 1)

	_ = conn.Close()
	waitForCount(t, srv.Hub, "ghost@example.com", 0)
}

func TestClientSendMessagePublishesCommand(t *testing.T) {
	newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "alice", "alice@example.com")

	srv, pub := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWS(t, ts.URL, authToken(t, user.Email, user.Login))
	defer conn.Close()
	waitForCount(t, srv.Hub, user.Email, 1)

	payload := map[string]any{
		"event": GatewayEventSendMessage,
		"data": map[string]any{
			"conversation_id": "",
			"recipient_email": "bob@example.com",
			"text":            "hello",
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := wsutil.WriteClientText(conn, raw); err != nil {
		t.Fatalf("write client text: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pub.mu.Lock()
		n := len(pub.sendCmds)
		pub.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected send command publish")
}

func TestServerStartRequiresNATSURL(t *testing.T) {
	newChatGatewayTestRepo(t)

	prev := os.Getenv("NATS_URL")
	if err := os.Unsetenv("NATS_URL"); err != nil {
		t.Fatalf("unset NATS_URL: %v", err)
	}
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv("NATS_URL")
			return
		}
		_ = os.Setenv("NATS_URL", prev)
	})

	srv := NewServer()
	err := srv.Start(context.Background(), "127.0.0.1:0")
	if err == nil || err.Error() != "NATS_URL is required for chat server" {
		t.Fatalf("expected NATS_URL requirement error, got %v", err)
	}
}
