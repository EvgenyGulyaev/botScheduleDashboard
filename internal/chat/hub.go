package chat

import (
	"botDashboard/internal/event"
	"botDashboard/internal/model"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*Client]struct{})}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.user.Email] == nil {
		h.clients[c.user.Email] = make(map[*Client]struct{})
	}
	h.clients[c.user.Email][c] = struct{}{}
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.clients[c.user.Email]
	if clients == nil {
		return
	}
	delete(clients, c)
	if len(clients) == 0 {
		delete(h.clients, c.user.Email)
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

func (h *Hub) HandleChatMessageReadUpdated(ev event.ChatMessageReadUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventMessageReadUpdated, ev)
}

func (h *Hub) HandleChatConversationUpdated(ev event.ChatConversationUpdatedEvent) {
	h.broadcast(ev.Members, GatewayEventConversationUpdated, ev)
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

type CommandPublisher interface {
	PublishChatMessageSendCommand(event.ChatMessageSendCommand) error
	PublishChatMessageReadCommand(event.ChatMessageReadCommand) error
}

type clientPublisher struct{}

func (clientPublisher) PublishChatMessageSendCommand(cmd event.ChatMessageSendCommand) error {
	return event.PublishChatMessageSendCommand(cmd)
}

func (clientPublisher) PublishChatMessageReadCommand(cmd event.ChatMessageReadCommand) error {
	return event.PublishChatMessageReadCommand(cmd)
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
	default:
		c.enqueue(GatewayEventError, gatewayErrorPayload{Message: fmt.Sprintf("unknown event %q", env.Event)})
	}
}
