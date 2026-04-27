package event

import (
	"botDashboard/pkg/broker"
	"botDashboard/pkg/shutdown"
	"context"
)

func RunBroker() {
	b := broker.Get()
	ctx, cancel := context.WithCancel(context.Background())
	userListener := broker.NewListener[User](ctx, b.Nc, "user", HandleUser)
	chatSendListener := broker.NewListener[ChatMessageSendCommand](ctx, b.Nc, ChatCommandMessageSend, HandleChatMessageSendCommand)
	chatReadListener := broker.NewListener[ChatMessageReadCommand](ctx, b.Nc, ChatCommandMessageRead, HandleChatMessageReadCommand)
	chatDeliveredListener := broker.NewListener[ChatMessageDeliveredCommand](ctx, b.Nc, ChatCommandMessageDelivered, HandleChatMessageDeliveredCommand)
	chatPresenceListener := broker.NewListener[ChatPresenceCommand](ctx, b.Nc, ChatCommandPresence, HandleChatPresenceCommand)
	chatTypingListener := broker.NewListener[ChatTypingCommand](ctx, b.Nc, ChatCommandTyping, HandleChatTypingCommand)

	// Запускаем shutdown для освобождения ресурсов, при перезапуске
	sd := shutdown.Get()
	sd.Add(cancel)
	sd.Add(userListener.Stop)
	sd.Add(chatSendListener.Stop)
	sd.Add(chatReadListener.Stop)
	sd.Add(chatDeliveredListener.Stop)
	sd.Add(chatPresenceListener.Stop)
	sd.Add(chatTypingListener.Stop)
	sd.Add(b.Close)
	go sd.Wait()
}
