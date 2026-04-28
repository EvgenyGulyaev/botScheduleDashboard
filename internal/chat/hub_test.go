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
	mu            sync.Mutex
	sendCmds      []event.ChatMessageSendCommand
	readCmds      []event.ChatMessageReadCommand
	deliveredCmds []event.ChatMessageDeliveredCommand
	presenceCmds  []event.ChatPresenceCommand
	typingCmds    []event.ChatTypingCommand
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

func (p *testCommandPublisher) PublishChatMessageDeliveredCommand(cmd event.ChatMessageDeliveredCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deliveredCmds = append(p.deliveredCmds, cmd)
	return nil
}

func (p *testCommandPublisher) PublishChatPresenceCommand(cmd event.ChatPresenceCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.presenceCmds = append(p.presenceCmds, cmd)
	return nil
}

func (p *testCommandPublisher) PublishChatTypingCommand(cmd event.ChatTypingCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.typingCmds = append(p.typingCmds, cmd)
	return nil
}

func decodeEnvelopeData[T any](t *testing.T, env gatewayEnvelope) T {
	t.Helper()

	var payload T
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("decode envelope payload: %v", err)
	}
	return payload
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

func readGatewayEnvelopeWithTimeout(t *testing.T, conn net.Conn, timeout time.Duration) (gatewayEnvelope, bool) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()
	msgs, err := wsutil.ReadServerMessage(conn, nil)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return gatewayEnvelope{}, false
		}
		t.Fatalf("read server message: %v", err)
	}
	var env gatewayEnvelope
	if err := json.Unmarshal(msgs[len(msgs)-1].Payload, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env, true
}

func readGatewayEnvelopeOfType(t *testing.T, conn net.Conn, eventName string) gatewayEnvelope {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		env, ok := readGatewayEnvelopeWithTimeout(t, conn, time.Until(deadline))
		if !ok {
			break
		}
		if env.Event == eventName {
			return env
		}
	}
	t.Fatalf("expected websocket event %s", eventName)
	return gatewayEnvelope{}
}

func assertNoGatewayEvent(t *testing.T, conn net.Conn, forbidden string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		env, ok := readGatewayEnvelopeWithTimeout(t, conn, time.Until(deadline))
		if !ok {
			return
		}
		if env.Event == forbidden {
			t.Fatalf("did not expect gateway event %s", forbidden)
		}
	}
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

func waitForTypingCommands(t *testing.T, pub *testCommandPublisher, want int) []event.ChatTypingCommand {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pub.mu.Lock()
		commands := append([]event.ChatTypingCommand(nil), pub.typingCmds...)
		pub.mu.Unlock()
		if len(commands) >= want {
			return commands
		}
		time.Sleep(10 * time.Millisecond)
	}
	pub.mu.Lock()
	commands := append([]event.ChatTypingCommand(nil), pub.typingCmds...)
	pub.mu.Unlock()
	t.Fatalf("expected %d typing commands, got %#v", want, commands)
	return nil
}

func waitForDeliveredCommands(t *testing.T, pub *testCommandPublisher, want int) []event.ChatMessageDeliveredCommand {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pub.mu.Lock()
		commands := append([]event.ChatMessageDeliveredCommand(nil), pub.deliveredCmds...)
		pub.mu.Unlock()
		if len(commands) >= want {
			return commands
		}
		time.Sleep(10 * time.Millisecond)
	}
	pub.mu.Lock()
	commands := append([]event.ChatMessageDeliveredCommand(nil), pub.deliveredCmds...)
	pub.mu.Unlock()
	t.Fatalf("expected %d delivered commands, got %#v", want, commands)
	return nil
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

func TestUnregisterIsIdempotentForSameClient(t *testing.T) {
	newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "alice", "alice@example.com")

	hub := NewHub()
	publisher := &testCommandPublisher{}
	firstServer, firstClient := net.Pipe()
	secondServer, secondClient := net.Pipe()
	t.Cleanup(func() {
		_ = firstServer.Close()
		_ = firstClient.Close()
		_ = secondServer.Close()
		_ = secondClient.Close()
	})

	first := newClient(firstServer, hub, user, publisher)
	second := newClient(secondServer, hub, user, publisher)
	hub.Register(first)
	hub.Register(second)

	hub.Unregister(first)
	hub.Unregister(first)
	if got := hub.ClientCount(user.Email); got != 1 {
		t.Fatalf("expected one registered client, got %d", got)
	}
	if len(publisher.presenceCmds) != 1 || !publisher.presenceCmds[0].Online {
		t.Fatalf("expected only one online presence command while second client remains, got %#v", publisher.presenceCmds)
	}

	hub.Unregister(second)
	if len(publisher.presenceCmds) != 2 || publisher.presenceCmds[1].Online {
		t.Fatalf("expected offline presence command after final client leaves, got %#v", publisher.presenceCmds)
	}
}

func TestWebsocketConnectDisconnectPublishPresenceCommands(t *testing.T) {
	newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "alice", "alice@example.com")

	srv, pub := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWS(t, ts.URL, authToken(t, user.Email, user.Login))
	waitForCount(t, srv.Hub, user.Email, 1)

	pub.mu.Lock()
	commands := append([]event.ChatPresenceCommand(nil), pub.presenceCmds...)
	pub.mu.Unlock()
	if len(commands) != 1 || !commands[0].Online || commands[0].UserEmail != user.Email {
		t.Fatalf("expected online presence command, got %#v", commands)
	}

	_ = conn.Close()
	waitForCount(t, srv.Hub, user.Email, 0)

	pub.mu.Lock()
	commands = append([]event.ChatPresenceCommand(nil), pub.presenceCmds...)
	pub.mu.Unlock()
	if len(commands) != 2 || commands[1].Online || commands[1].UserEmail != user.Email {
		t.Fatalf("expected offline presence command, got %#v", commands)
	}
}

func TestWebsocketConnectDoesNotWritePresenceInGatewayProcess(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "alice", "alice@example.com")

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWS(t, ts.URL, authToken(t, user.Email, user.Login))
	waitForCount(t, srv.Hub, user.Email, 1)

	presence, err := repo.UserPresence(user.Email)
	if err != nil {
		t.Fatalf("load presence after connect: %v", err)
	}
	if repo.IsUserOnline(user.Email) || !presence.LastActiveAt.IsZero() {
		t.Fatalf("expected gateway to avoid direct presence DB writes, got online=%v presence=%#v", repo.IsUserOnline(user.Email), presence)
	}

	_ = conn.Close()
	waitForCount(t, srv.Hub, user.Email, 0)
	presence, err = repo.UserPresence(user.Email)
	if err != nil {
		t.Fatalf("load presence after disconnect: %v", err)
	}
	if repo.IsUserOnline(user.Email) || !presence.LastSeenAt.IsZero() {
		t.Fatalf("expected gateway to keep presence persistence in command consumer, got online=%v presence=%#v", repo.IsUserOnline(user.Email), presence)
	}
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

	aliceEnv := readGatewayEnvelopeOfType(t, aliceConn, GatewayEventMessagePersisted)
	if aliceEnv.Event != GatewayEventMessagePersisted {
		t.Fatalf("unexpected alice event: %#v", aliceEnv)
	}
	bobEnv := readGatewayEnvelopeOfType(t, bobConn, GatewayEventMessagePersisted)
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

	first := readGatewayEnvelopeOfType(t, aliceConn, GatewayEventMessageReadUpdated)
	if first.Event != GatewayEventMessageReadUpdated {
		t.Fatalf("unexpected first event: %#v", first)
	}
	second := readGatewayEnvelopeOfType(t, aliceConn, GatewayEventConversationUpdated)
	if second.Event != GatewayEventConversationUpdated {
		t.Fatalf("unexpected second event: %#v", second)
	}
}

func TestPresenceUpdatedFansOutOnlyToConversationMembers(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")
	carol := createGatewayUser(t, "carol", "carol@example.com")
	dave := createGatewayUser(t, "dave", "dave@example.com")

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: alice.Email, Login: alice.Login},
		model.ChatMember{Email: bob.Email, Login: bob.Login},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := repo.CreateDirectConversation(
		model.ChatMember{Email: carol.Email, Login: carol.Login},
		model.ChatMember{Email: dave.Email, Login: dave.Login},
	); err != nil {
		t.Fatalf("create second conversation: %v", err)
	}

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	bobConn := dialChatWS(t, ts.URL, authToken(t, bob.Email, bob.Login))
	carolConn := dialChatWS(t, ts.URL, authToken(t, carol.Email, carol.Login))
	waitForCount(t, srv.Hub, bob.Email, 1)
	waitForCount(t, srv.Hub, carol.Email, 1)

	srv.Hub.HandleChatPresenceUpdated(event.ChatPresenceUpdatedEvent{
		ConversationID: conv.ID,
		Members: []model.ChatMember{
			{ConversationID: conv.ID, Email: alice.Email, Login: alice.Login},
			{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login},
		},
		User:     event.ChatParticipant{Email: alice.Email, Login: alice.Login},
		Presence: model.ChatUserPresence{Email: alice.Email, Login: alice.Login, Online: true, LastActiveAt: time.Now().UTC()},
	})

	bobEnv := readGatewayEnvelopeOfType(t, bobConn, GatewayEventPresenceUpdated)
	if bobEnv.Event != GatewayEventPresenceUpdated {
		t.Fatalf("expected bob presence event, got %#v", bobEnv)
	}
	if _, ok := readGatewayEnvelopeWithTimeout(t, carolConn, 100*time.Millisecond); ok {
		t.Fatal("did not expect unrelated conversation member to receive presence event")
	}
}

func TestTypingCommandsMutateStateAndFanOutToConversationPeers(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")
	carol := createGatewayUser(t, "carol", "carol@example.com")

	conv, err := repo.CreateDirectConversation(
		model.ChatMember{Email: alice.Email, Login: alice.Login},
		model.ChatMember{Email: bob.Email, Login: bob.Login},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	other, err := repo.CreateDirectConversation(
		model.ChatMember{Email: alice.Email, Login: alice.Login},
		model.ChatMember{Email: carol.Email, Login: carol.Login},
	)
	if err != nil {
		t.Fatalf("create other conversation: %v", err)
	}

	srv, pub := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	aliceConn := dialChatWS(t, ts.URL, authToken(t, alice.Email, alice.Login))
	bobConn := dialChatWS(t, ts.URL, authToken(t, bob.Email, bob.Login))
	carolConn := dialChatWS(t, ts.URL, authToken(t, carol.Email, carol.Login))
	waitForCount(t, srv.Hub, alice.Email, 1)
	waitForCount(t, srv.Hub, bob.Email, 1)
	waitForCount(t, srv.Hub, carol.Email, 1)

	raw, err := json.Marshal(map[string]any{
		"event": GatewayEventTypingStarted,
		"data": map[string]any{
			"conversation_id": conv.ID,
		},
	})
	if err != nil {
		t.Fatalf("marshal typing payload: %v", err)
	}
	if err := wsutil.WriteClientText(aliceConn, raw); err != nil {
		t.Fatalf("write typing start: %v", err)
	}
	typingCmds := waitForTypingCommands(t, pub, 1)
	srv.Hub.HandleChatTypingEvent(event.ChatTypingEvent{
		ConversationID: conv.ID,
		Members:        []model.ChatMember{{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login}},
		User:           event.ChatParticipant{Email: typingCmds[0].UserEmail, Login: typingCmds[0].UserLogin},
		Kind:           typingCmds[0].Kind,
		StartedAt:      time.Now().UTC(),
	})

	bobEnv := readGatewayEnvelopeOfType(t, bobConn, GatewayEventTypingStarted)
	if bobEnv.Event != GatewayEventTypingStarted {
		t.Fatalf("expected typing_start for bob, got %#v", bobEnv)
	}
	assertNoGatewayEvent(t, aliceConn, GatewayEventTypingStarted, 100*time.Millisecond)
	assertNoGatewayEvent(t, carolConn, GatewayEventTypingStarted, 100*time.Millisecond)
	if typers := srv.Hub.ActiveTypers(conv.ID); len(typers) != 1 || typers[0].Email != alice.Email {
		t.Fatalf("expected alice active typer, got %#v", typers)
	}
	if typers := srv.Hub.ActiveTypers(other.ID); len(typers) != 0 {
		t.Fatalf("expected no typers in other conversation, got %#v", typers)
	}

	raw, err = json.Marshal(map[string]any{
		"event": GatewayEventTypingStopped,
		"data": map[string]any{
			"conversation_id": conv.ID,
		},
	})
	if err != nil {
		t.Fatalf("marshal typing stop payload: %v", err)
	}
	if err := wsutil.WriteClientText(aliceConn, raw); err != nil {
		t.Fatalf("write typing stop: %v", err)
	}
	typingCmds = waitForTypingCommands(t, pub, 2)
	srv.Hub.HandleChatTypingEvent(event.ChatTypingEvent{
		ConversationID: conv.ID,
		Members:        []model.ChatMember{{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login}},
		User:           event.ChatParticipant{Email: typingCmds[1].UserEmail, Login: typingCmds[1].UserLogin},
		Kind:           typingCmds[1].Kind,
		StartedAt:      time.Now().UTC(),
	})
	stopEnv := readGatewayEnvelopeOfType(t, bobConn, GatewayEventTypingStopped)
	if stopEnv.Event != GatewayEventTypingStopped {
		t.Fatalf("expected typing_stop for bob, got %#v", stopEnv)
	}
	if typers := srv.Hub.ActiveTypers(conv.ID); len(typers) != 0 {
		t.Fatalf("expected typing state to clear, got %#v", typers)
	}
	if len(typingCmds) != 2 || typingCmds[0].Kind != "started" || typingCmds[1].Kind != "stopped" {
		t.Fatalf("expected typing start/stop commands, got %#v", typingCmds)
	}
}

func TestTypingStateExpiresWhenStopNeverArrives(t *testing.T) {
	hub := NewHub()
	hub.typingTTL = 30 * time.Millisecond
	hub.SetTyping(event.ChatTypingCommand{
		ConversationID: "conv-1",
		UserEmail:      "alice@example.com",
		UserLogin:      "alice",
		Kind:           "started",
	})
	if typers := hub.ActiveTypers("conv-1"); len(typers) != 1 {
		t.Fatalf("expected active typer before expiry, got %#v", typers)
	}
	time.Sleep(50 * time.Millisecond)
	hub.PruneExpiredTyping()
	if typers := hub.ActiveTypers("conv-1"); len(typers) != 0 {
		t.Fatalf("expected expired typing state, got %#v", typers)
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
			"conversation_id":   "",
			"recipient_email":   "bob@example.com",
			"text":              "hello",
			"client_message_id": "client-1",
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
		commands := append([]event.ChatMessageSendCommand(nil), pub.sendCmds...)
		pub.mu.Unlock()
		if len(commands) > 0 {
			if commands[0].ClientMessageID != "client-1" {
				t.Fatalf("expected client message id in send command, got %#v", commands[0])
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected send command publish")
}

func TestClientMessageReceivedPublishesDeliveredCommand(t *testing.T) {
	newChatGatewayTestRepo(t)
	user := createGatewayUser(t, "bob", "bob@example.com")

	srv, pub := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	conn := dialChatWS(t, ts.URL, authToken(t, user.Email, user.Login))
	defer conn.Close()
	waitForCount(t, srv.Hub, user.Email, 1)

	raw, err := json.Marshal(map[string]any{
		"event": GatewayEventMessageReceived,
		"data": map[string]any{
			"conversation_id": "conv-1",
			"message_id":      "msg-1",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := wsutil.WriteClientText(conn, raw); err != nil {
		t.Fatalf("write client text: %v", err)
	}

	commands := waitForDeliveredCommands(t, pub, 1)
	if commands[0].ConversationID != "conv-1" || commands[0].MessageID != "msg-1" || commands[0].RecipientEmail != user.Email || commands[0].RecipientLogin != user.Login {
		t.Fatalf("unexpected delivered command: %#v", commands[0])
	}
}

func TestHubRoutesMessageDeliveredEvents(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")

	conv, err := repo.CreateDirectConversation(model.ChatMember{Email: alice.Email, Login: alice.Login}, model.ChatMember{Email: bob.Email, Login: bob.Login})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	msg, err := repo.AddMessageWithClientMessageID(conv.ID, alice.Email, alice.Login, "hello", "client-1", "")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}
	msg, _, err = repo.MarkMessageDelivered(conv.ID, msg.ID, bob.Email, bob.Login)
	if err != nil {
		t.Fatalf("mark delivered: %v", err)
	}

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	aliceConn := dialChatWS(t, ts.URL, authToken(t, alice.Email, alice.Login))
	waitForCount(t, srv.Hub, alice.Email, 1)

	srv.Hub.HandleChatMessageDelivered(event.ChatMessageDeliveredEvent{
		Conversation: conv,
		Members: []model.ChatMember{
			{ConversationID: conv.ID, Email: alice.Email, Login: alice.Login},
			{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login},
		},
		MessageID: msg.ID,
		Message:   msg,
		Recipient: event.ChatParticipant{Email: bob.Email, Login: bob.Login},
	})

	env := readGatewayEnvelopeOfType(t, aliceConn, GatewayEventMessageDelivered)
	if env.Event != GatewayEventMessageDelivered {
		t.Fatalf("unexpected delivered event: %#v", env)
	}
	payload := decodeEnvelopeData[event.ChatMessageDeliveredEvent](t, env)
	if payload.Message.ClientMessageID != "client-1" || payload.Message.DeliveryStatus != "delivered" {
		t.Fatalf("expected reconciliation and delivery metadata in payload, got %#v", payload.Message)
	}
}

func TestHubRoutesCallEventsToConnectedParticipants(t *testing.T) {
	repo := newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")

	conv, err := repo.CreateDirectConversation(model.ChatMember{Email: alice.Email, Login: alice.Login}, model.ChatMember{Email: bob.Email, Login: bob.Login})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	call, message, _, err := repo.StartCall(conv.ID, alice.Email, alice.Login)
	if err != nil {
		t.Fatalf("start call: %v", err)
	}

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	aliceConn := dialChatWS(t, ts.URL, authToken(t, alice.Email, alice.Login))
	bobConn := dialChatWS(t, ts.URL, authToken(t, bob.Email, bob.Login))
	waitForCount(t, srv.Hub, alice.Email, 1)
	waitForCount(t, srv.Hub, bob.Email, 1)

	payload := event.ChatCallStartedEvent{
		Conversation: conv,
		Members: []model.ChatMember{
			{ConversationID: conv.ID, Email: alice.Email, Login: alice.Login},
			{ConversationID: conv.ID, Email: bob.Email, Login: bob.Login},
		},
		Call:    call,
		Message: message,
	}
	srv.Hub.HandleChatCallStarted(payload)

	aliceEnv := readGatewayEnvelopeOfType(t, aliceConn, GatewayEventCallStarted)
	if aliceEnv.Event != GatewayEventCallStarted {
		t.Fatalf("unexpected alice call event: %#v", aliceEnv)
	}
	bobEnv := readGatewayEnvelopeOfType(t, bobConn, GatewayEventCallStarted)
	if bobEnv.Event != GatewayEventCallStarted {
		t.Fatalf("unexpected bob call event: %#v", bobEnv)
	}
}

func TestClientCallSignalRoutesToRecipient(t *testing.T) {
	newChatGatewayTestRepo(t)
	alice := createGatewayUser(t, "alice", "alice@example.com")
	bob := createGatewayUser(t, "bob", "bob@example.com")

	srv, _ := newChatGatewayServer(t)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	aliceConn := dialChatWS(t, ts.URL, authToken(t, alice.Email, alice.Login))
	bobConn := dialChatWS(t, ts.URL, authToken(t, bob.Email, bob.Login))
	waitForCount(t, srv.Hub, alice.Email, 1)
	waitForCount(t, srv.Hub, bob.Email, 1)

	raw, err := json.Marshal(map[string]any{
		"event": GatewayEventCallSignal,
		"data": map[string]any{
			"call_id":         "call-1",
			"conversation_id": "conv-1",
			"recipient_email": bob.Email,
			"kind":            "offer",
			"payload":         map[string]any{"sdp": "fake-sdp"},
		},
	})
	if err != nil {
		t.Fatalf("marshal call signal: %v", err)
	}
	if err := wsutil.WriteClientText(aliceConn, raw); err != nil {
		t.Fatalf("write client text: %v", err)
	}

	bobEnv := readGatewayEnvelopeOfType(t, bobConn, GatewayEventCallSignal)
	if bobEnv.Event != GatewayEventCallSignal {
		t.Fatalf("unexpected bob signal event: %#v", bobEnv)
	}
	payload := decodeEnvelopeData[gatewayCallSignalPayload](t, bobEnv)
	if payload.SenderEmail != alice.Email || payload.RecipientEmail != bob.Email || payload.Kind != "offer" {
		t.Fatalf("unexpected signal payload: %#v", payload)
	}
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
