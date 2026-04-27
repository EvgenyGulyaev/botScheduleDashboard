package consumer

import "botDashboard/internal/event"

type ChatMessageSendCommand = event.ChatMessageSendCommand
type ChatMessageReadCommand = event.ChatMessageReadCommand
type ChatMessageDeliveredCommand = event.ChatMessageDeliveredCommand

func HandleChatMessageSend(cmd ChatMessageSendCommand) {
	event.HandleChatMessageSendCommand(cmd)
}

func HandleChatMessageRead(cmd ChatMessageReadCommand) {
	event.HandleChatMessageReadCommand(cmd)
}

func HandleChatMessageDelivered(cmd ChatMessageDeliveredCommand) {
	event.HandleChatMessageDeliveredCommand(cmd)
}
