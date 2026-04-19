package event

import (
	"botDashboard/pkg/broker"
	"sync"
)

type Publisher interface {
	Publish(subject string, payload any) error
}

type natsPublisher struct{}

func (natsPublisher) Publish(subject string, payload any) error {
	b := broker.Get()
	return broker.Publish(b.Nc, subject, payload)
}

var (
	publisherMu sync.RWMutex
	publisher   Publisher = natsPublisher{}
)

func SetPublisherForTest(p Publisher) {
	publisherMu.Lock()
	defer publisherMu.Unlock()
	publisher = p
}

func ResetPublisherForTest() {
	SetPublisherForTest(natsPublisher{})
}

func publish(subject string, payload any) error {
	publisherMu.RLock()
	current := publisher
	publisherMu.RUnlock()
	return current.Publish(subject, payload)
}

func PublishChatMessageSendCommand(payload ChatMessageSendCommand) error {
	return publish(ChatCommandMessageSend, payload)
}

func PublishChatMessageReadCommand(payload ChatMessageReadCommand) error {
	return publish(ChatCommandMessageRead, payload)
}

func PublishChatMessagePersistedEvent(payload ChatMessagePersistedEvent) error {
	return publish(ChatEventMessagePersisted, payload)
}

func PublishChatMessageUpdatedEvent(payload ChatMessageUpdatedEvent) error {
	return publish(ChatEventMessageUpdated, payload)
}

func PublishChatMessageDeletedEvent(payload ChatMessageDeletedEvent) error {
	return publish(ChatEventMessageDeleted, payload)
}

func PublishChatMessageReadUpdatedEvent(payload ChatMessageReadUpdatedEvent) error {
	return publish(ChatEventMessageReadUpdated, payload)
}

func PublishChatConversationUpdatedEvent(payload ChatConversationUpdatedEvent) error {
	return publish(ChatEventConversationUpdated, payload)
}
