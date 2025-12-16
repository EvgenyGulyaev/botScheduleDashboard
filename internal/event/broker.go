package event

import (
	"botDashboard/internal/event/consumer"
	"botDashboard/pkg/broker"
	"botDashboard/pkg/shutdown"
	"context"
)

func RunBroker() {
	b := broker.Get()
	ctx, cancel := context.WithCancel(context.Background())
	userListener := broker.NewListener[consumer.User](ctx, b.Nc, "user", consumer.HandleUser)

	// Запускаем shutdown для освобождения ресурсов, при перезапуске
	sd := shutdown.Get()
	sd.Add(cancel)
	sd.Add(userListener.Stop)
	sd.Add(b.Close)
	go sd.Wait()
}
