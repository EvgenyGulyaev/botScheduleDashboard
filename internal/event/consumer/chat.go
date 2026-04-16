package consumer

import "botDashboard/internal/event"

type ChatMessageSendCommand = event.ChatMessageSendCommand
type ChatMessageReadCommand = event.ChatMessageReadCommand

func HandleChatMessageSend(cmd ChatMessageSendCommand) {
	event.HandleChatMessageSendCommand(cmd)
}

func HandleChatMessageRead(cmd ChatMessageReadCommand) {
	event.HandleChatMessageReadCommand(cmd)
}
