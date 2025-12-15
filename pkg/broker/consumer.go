package broker

import (
	"context"
	"log"

	"github.com/nats-io/nats.go"
)

type Listener[T any] struct {
	ctx       context.Context
	nc        *nats.Conn
	cancelSub context.CancelFunc
	handler   func(T)
}

func NewListener[T any](ctx context.Context, nc *nats.Conn, subject string, handler func(T)) *Listener[T] {
	ol := &Listener[T]{
		ctx:     ctx,
		nc:      nc,
		handler: handler,
	}
	ol.Start(subject)
	return ol
}

func (ol *Listener[T]) Start(subject string) {
	ch, cancelSub, err := Subscribe[T](ol.nc, subject, 100, ol.ctx)
	if err != nil {
		log.Printf("subscribe error: %v", err)
		return
	}
	ol.cancelSub = cancelSub

	go func() {
		defer cancelSub()
		for msg := range ch {
			ol.handler(msg)
		}
	}()
}

func (ol *Listener[T]) Stop() {
	if ol.cancelSub != nil {
		ol.cancelSub()
	}
}
