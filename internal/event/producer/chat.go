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

func PublishChatMessageReadUpdatedEvent(payload event.ChatMessageReadUpdatedEvent) error {
	return event.PublishChatMessageReadUpdatedEvent(payload)
}

func PublishChatConversationUpdatedEvent(payload event.ChatConversationUpdatedEvent) error {
	return event.PublishChatConversationUpdatedEvent(payload)
}
