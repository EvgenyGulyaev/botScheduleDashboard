package chat

import (
	"encoding/json"

	"botDashboard/internal/event"
)

const (
	GatewayEventSendMessage         = "send_message"
	GatewayEventMarkRead            = "mark_read"
	GatewayEventPing                = "ping"
	GatewayEventMessagePersisted    = "message_persisted"
	GatewayEventMessageUpdated      = "message_updated"
	GatewayEventMessageDeleted      = "message_deleted"
	GatewayEventMessageReadUpdated  = "message_read_updated"
	GatewayEventConversationUpdated = "conversation_updated"
	GatewayEventPong                = "pong"
	GatewayEventError               = "error"
)

type gatewayEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type gatewayErrorPayload struct {
	Message string `json:"message"`
}

type gatewayPongPayload struct {
	Message string `json:"message,omitempty"`
}

type gatewaySendMessagePayload = event.ChatMessageSendCommand
type gatewayMarkReadPayload = event.ChatMessageReadCommand

type gatewayMessagePersistedPayload = event.ChatMessagePersistedEvent
type gatewayMessageUpdatedPayload = event.ChatMessageUpdatedEvent
type gatewayMessageDeletedPayload = event.ChatMessageDeletedEvent
type gatewayMessageReadUpdatedPayload = event.ChatMessageReadUpdatedEvent
type gatewayConversationUpdatedPayload = event.ChatConversationUpdatedEvent

func encodeGatewayEvent(name string, payload any) ([]byte, error) {
	if payload == nil {
		return json.Marshal(gatewayEnvelope{Event: name})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(gatewayEnvelope{Event: name, Data: raw})
}

func decodeGatewayEnvelope(data []byte) (gatewayEnvelope, error) {
	var env gatewayEnvelope
	err := json.Unmarshal(data, &env)
	return env, err
}
