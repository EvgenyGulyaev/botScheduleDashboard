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

	// Запускаем shutdown для освобождения ресурсов, при перезапуске
	sd := shutdown.Get()
	sd.Add(cancel)
	sd.Add(userListener.Stop)
	sd.Add(chatSendListener.Stop)
	sd.Add(chatReadListener.Stop)
	sd.Add(b.Close)
	go sd.Wait()
}
