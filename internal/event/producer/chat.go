package producer

import "botDashboard/internal/event"

func SetPublisherForTest(p event.Publisher) {
	event.SetPublisherForTest(p)
}

func ResetPublisherForTest() {
	event.ResetPublisherForTest()
}

func PublishChatMessageSendCommand(payload event.ChatMessageSendCommand) error {
	return event.PublishChatMessageSendCommand(payload)
}

func PublishChatMessageReadCommand(payload event.ChatMessageReadCommand) error {
	return event.PublishChatMessageReadCommand(payload)
}

func PublishChatMessagePersistedEvent(payload event.ChatMessagePersistedEvent) error {
	return event.PublishChatMessagePersistedEvent(payload)
}

func PublishChatMessageUpdatedEvent(payload event.ChatMessageUpdatedEvent) error {
	return event.PublishChatMessageUpdatedEvent(payload)
}

func PublishChatMessageDeletedEvent(payload event.ChatMessageDeletedEvent) error {
	return event.PublishChatMessageDeletedEvent(payload)
}

func PublishChatMessageReadUpdatedEvent(payload event.ChatMessageReadUpdatedEvent) error {
	return event.PublishChatMessageReadUpdatedEvent(payload)
}

func PublishChatConversationUpdatedEvent(payload event.ChatConversationUpdatedEvent) error {
	return event.PublishChatConversationUpdatedEvent(payload)
}

func PublishChatCallStartedEvent(payload event.ChatCallStartedEvent) error {
	return event.PublishChatCallStartedEvent(payload)
}

func PublishChatCallUpdatedEvent(payload event.ChatCallUpdatedEvent) error {
	return event.PublishChatCallUpdatedEvent(payload)
}

func PublishChatCallEndedEvent(payload event.ChatCallEndedEvent) error {
	return event.PublishChatCallEndedEvent(payload)
}
