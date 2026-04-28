package chat

import (
	"botDashboard/internal/event"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

type Hub struct {
	mu        sync.RWMutex
	clients   map[string]map[*Client]struct{}
	typing    map[string]map[string]typingState
	typingTTL time.Duration
}

type typingState struct {
	user      model.ChatTypingUser
	expiresAt time.Time
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[string]map[*Client]struct{}),
		typing:    make(map[string]map[string]typingState),
		typingTTL: 6 * time.Second,
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	transitioned := len(h.clients[c.user.Email]) == 0
	if h.clients[c.user.Email] == nil {
		h.clients[c.user.Email] = make(map[*Client]struct{})
	}
	h.clients[c.user.Email][c] = struct{}{}
	h.mu.Unlock()

	if transitioned {
		_ = c.publisher.PublishChatPresenceCommand(event.ChatPresenceCommand{
			UserEmail: c.user.Email,
			UserLogin: c.user.Login,
			Online:    true,
		})
	}
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	clients := h.clients[c.user.Email]
	if clients == nil {
		h.mu.Unlock()
		return
	}
	if _, ok := clients[c]; !ok {
		h.mu.Unlock()
		return
	}
	delete(clients, c)
	transitioned := len(clients) == 0
	if transitioned {
		delete(h.clients, c.user.Email)
	}
	h.mu.Unlock()

	if transitioned {
		_ = c.publisher.PublishChatPresenceCommand(event.ChatPresenceCommand{
			UserEmail: c.user.Email,
			UserLogin: c.user.Login,
			Online:    false,
		})
	}
}

func (h *Hub) ClientCount(email string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[email])
}

func (h *Hub) HandleChatMessagePersisted(ev event.ChatMessagePersistedEvent) {
	h.broadcast(ev.Members, GatewayEventMessagePersisted, ev)
}

func (h *Hub) HandleChatMessageDelivered(ev event.ChatMessageDeliveredEvent) {
	h.broadcast(ev.Members, GatewayEventMessageDelivered, ev)
}

func (h *Hub) HandleChatMessageUpdated(ev event.ChatMessageUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventMessageUpdated, ev)
}

func (h *Hub) HandleChatMessageDeleted(ev event.ChatMessageDeletedEvent) {
	h.broadcast(ev.Members, GatewayEventMessageDeleted, ev)
}

func (h *Hub) HandleChatMessageReadUpdated(ev event.ChatMessageReadUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventMessageReadUpdated, ev)
}

func (h *Hub) HandleChatConversationUpdated(ev event.ChatConversationUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventConversationUpdated, ev)
}

func (h *Hub) HandleChatPresenceUpdated(ev event.ChatPresenceUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventPresenceUpdated, ev)
}

func (h *Hub) HandleChatTypingEvent(ev event.ChatTypingEvent) {
	h.applyTypingEvent(ev)
	h.broadcast(ev.Members, typingGatewayEvent(ev.Kind), ev)
}

func (h *Hub) HandleChatCallStarted(ev event.ChatCallStartedEvent) {
	h.broadcast(ev.Members, GatewayEventCallStarted, ev)
}

func (h *Hub) HandleChatCallUpdated(ev event.ChatCallUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventCallUpdated, ev)
}

func (h *Hub) HandleChatCallEnded(ev event.ChatCallEndedEvent) {
	h.broadcast(ev.Members, GatewayEventCallEnded, ev)
}

func (h *Hub) HandleCallSignal(payload gatewayCallSignalPayload) {
	if payload.RecipientEmail == "" {
		return
	}
	data, err := encodeGatewayEvent(GatewayEventCallSignal, payload)
	if err != nil {
		return
	}
	for _, client := range h.snapshotClients([]model.ChatMember{{Email: payload.RecipientEmail}}) {
		select {
		case client.send <- data:
		default:
			client.Close()
			h.Unregister(client)
		}
	}
}

func (h *Hub) HandleLocalTypingCommand(cmd event.ChatTypingCommand) {
	ev, ok := h.typingEventForCommand(cmd)
	if !ok {
		return
	}
	h.HandleChatTypingEvent(ev)
}

func (h *Hub) SetTyping(cmd event.ChatTypingCommand) {
	if cmd.ConversationID == "" || cmd.UserEmail == "" || (cmd.Kind != "started" && cmd.Kind != "stopped") {
		return
	}
	h.applyTypingEvent(event.ChatTypingEvent{
		ConversationID: cmd.ConversationID,
		User:           event.ChatParticipant{Email: cmd.UserEmail, Login: cmd.UserLogin},
		Kind:           cmd.Kind,
		StartedAt:      time.Now().UTC(),
	})
}

func (h *Hub) ActiveTypers(conversationID string) []model.ChatTypingUser {
	h.PruneExpiredTyping()
	h.mu.RLock()
	defer h.mu.RUnlock()

	states := h.typing[conversationID]
	result := make([]model.ChatTypingUser, 0, len(states))
	for _, state := range states {
		result = append(result, state.user)
	}
	return result
}

func (h *Hub) PruneExpiredTyping() {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	for conversationID, states := range h.typing {
		for email, state := range states {
			if now.After(state.expiresAt) {
				delete(states, email)
			}
		}
		if len(states) == 0 {
			delete(h.typing, conversationID)
		}
	}
}

func (h *Hub) broadcast(members []model.ChatMember, name string, payload any) {
	data, err := encodeGatewayEvent(name, payload)
	if err != nil {
		return
	}

	clients := h.snapshotClients(members)
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			c.Close()
			h.Unregister(c)
		}
	}
}

func (h *Hub) snapshotClients(members []model.ChatMember) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*Client, 0)
	seen := make(map[*Client]struct{})
	for _, member := range members {
		for client := range h.clients[member.Email] {
			if _, ok := seen[client]; ok {
				continue
			}
			seen[client] = struct{}{}
			result = append(result, client)
		}
	}
	return result
}

func (h *Hub) typingEventForCommand(cmd event.ChatTypingCommand) (event.ChatTypingEvent, bool) {
	if cmd.ConversationID == "" || cmd.UserEmail == "" {
		return event.ChatTypingEvent{}, false
	}
	if cmd.Kind != "started" && cmd.Kind != "stopped" {
		return event.ChatTypingEvent{}, false
	}
	members, err := store.GetChatRepository().ListConversationMembers(cmd.ConversationID)
	if err != nil {
		return event.ChatTypingEvent{}, false
	}
	recipients := make([]model.ChatMember, 0, len(members))
	isMember := false
	for _, member := range members {
		if member.Email == cmd.UserEmail {
			isMember = true
			continue
		}
		recipients = append(recipients, member)
	}
	if !isMember {
		return event.ChatTypingEvent{}, false
	}
	return event.ChatTypingEvent{
		ConversationID: cmd.ConversationID,
		Members:        recipients,
		User:           event.ChatParticipant{Email: cmd.UserEmail, Login: cmd.UserLogin},
		Kind:           cmd.Kind,
		StartedAt:      time.Now().UTC(),
	}, true
}

func (h *Hub) applyTypingEvent(ev event.ChatTypingEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ev.Kind == "stopped" {
		if h.typing[ev.ConversationID] != nil {
			delete(h.typing[ev.ConversationID], ev.User.Email)
			if len(h.typing[ev.ConversationID]) == 0 {
				delete(h.typing, ev.ConversationID)
			}
		}
		return
	}
	startedAt := ev.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if h.typing[ev.ConversationID] == nil {
		h.typing[ev.ConversationID] = make(map[string]typingState)
	}
	h.typing[ev.ConversationID][ev.User.Email] = typingState{
		user: model.ChatTypingUser{
			Email:     ev.User.Email,
			Login:     ev.User.Login,
			StartedAt: startedAt,
		},
		expiresAt: time.Now().UTC().Add(h.typingTTL),
	}
}

func typingGatewayEvent(kind string) string {
	if kind == "stopped" {
		return GatewayEventTypingStopped
	}
	return GatewayEventTypingStarted
}

type CommandPublisher interface {
	PublishChatMessageSendCommand(event.ChatMessageSendCommand) error
	PublishChatMessageReadCommand(event.ChatMessageReadCommand) error
	PublishChatMessageDeliveredCommand(event.ChatMessageDeliveredCommand) error
	PublishChatPresenceCommand(event.ChatPresenceCommand) error
	PublishChatTypingCommand(event.ChatTypingCommand) error
}

type clientPublisher struct{}

func (clientPublisher) PublishChatMessageSendCommand(cmd event.ChatMessageSendCommand) error {
	return event.PublishChatMessageSendCommand(cmd)
}

func (clientPublisher) PublishChatMessageReadCommand(cmd event.ChatMessageReadCommand) error {
	return event.PublishChatMessageReadCommand(cmd)
}

func (clientPublisher) PublishChatMessageDeliveredCommand(cmd event.ChatMessageDeliveredCommand) error {
	return event.PublishChatMessageDeliveredCommand(cmd)
}

func (clientPublisher) PublishChatPresenceCommand(cmd event.ChatPresenceCommand) error {
	return event.PublishChatPresenceCommand(cmd)
}

func (clientPublisher) PublishChatTypingCommand(cmd event.ChatTypingCommand) error {
	return event.PublishChatTypingCommand(cmd)
}

type Client struct {
	user      model.UserData
	hub       *Hub
	conn      net.Conn
	send      chan []byte
	closeOnce sync.Once
	publisher CommandPublisher
}

func newClient(conn net.Conn, hub *Hub, user model.UserData, publisher CommandPublisher) *Client {
	if publisher == nil {
		publisher = clientPublisher{}
	}
	return &Client{
		user:      user,
		hub:       hub,
		conn:      conn,
		send:      make(chan []byte, 8),
		publisher: publisher,
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.send)
		_ = c.conn.Close()
	})
}

func (c *Client) enqueue(name string, payload any) {
	data, err := encodeGatewayEvent(name, payload)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

func (c *Client) handleIncoming(raw []byte) {
	env, err := decodeGatewayEnvelope(raw)
	if err != nil {
		c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
		return
	}

	switch env.Event {
	case GatewayEventPing:
		c.enqueue(GatewayEventPong, gatewayPongPayload{Message: "pong"})
	case GatewayEventSendMessage:
		var payload gatewaySendMessagePayload
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
			return
		}
		payload.SenderEmail = c.user.Email
		payload.SenderLogin = c.user.Login
		if err := c.publisher.PublishChatMessageSendCommand(event.ChatMessageSendCommand(payload)); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
		}
	case GatewayEventMarkRead:
		var payload gatewayMarkReadPayload
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
			return
		}
		payload.ReaderEmail = c.user.Email
		payload.ReaderLogin = c.user.Login
		if err := c.publisher.PublishChatMessageReadCommand(event.ChatMessageReadCommand(payload)); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
		}
	case GatewayEventMessageReceived:
		var payload gatewayMessageReceivedPayload
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
			return
		}
		payload.RecipientEmail = c.user.Email
		payload.RecipientLogin = c.user.Login
		if err := c.publisher.PublishChatMessageDeliveredCommand(event.ChatMessageDeliveredCommand(payload)); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
		}
	case GatewayEventTypingStarted, GatewayEventTypingStopped:
		var payload gatewayTypingPayload
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
			return
		}
		payload.UserEmail = c.user.Email
		payload.UserLogin = c.user.Login
		if env.Event == GatewayEventTypingStarted {
			payload.Kind = "started"
		} else {
			payload.Kind = "stopped"
		}
		if err := c.publisher.PublishChatTypingCommand(event.ChatTypingCommand(payload)); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
		}
	case GatewayEventCallSignal:
		var payload gatewayCallSignalPayload
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			c.enqueue(GatewayEventError, gatewayErrorPayload{Message: err.Error()})
			return
		}
		payload.SenderEmail = c.user.Email
		payload.SenderLogin = c.user.Login
		c.hub.HandleCallSignal(payload)
	default:
		c.enqueue(GatewayEventError, gatewayErrorPayload{Message: fmt.Sprintf("unknown event %q", env.Event)})
	}
}
