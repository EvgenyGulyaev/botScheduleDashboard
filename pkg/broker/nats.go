package broker

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/nats-io/nats.go"
)

type NatsBroker struct {
	nc *nats.Conn
}

func (b *NatsBroker) Close() {
	b.nc.Close()
}

func NewNatsBroker() *NatsBroker {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		return nil
	}
	return &NatsBroker{nc: nc}
}

func Publish[T any](nc *nats.Conn, subject string, d T) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return nc.Publish(subject, data)
}

func Subscribe[T any](nc *nats.Conn, subject string, bufferSize int, ctx context.Context) (<-chan T, context.CancelFunc, error) {
	ch := make(chan T, bufferSize)
	var once sync.Once // гарантирует вызов Unsubscribe только один раз

	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		var v T
		if err := json.Unmarshal(msg.Data, &v); err != nil {
			return
		}
		select {
		case ch <- v:
		case <-ctx.Done():
		}
	})
	if err != nil {
		close(ch)
		return nil, nil, err
	}

	cancel := func() {
		once.Do(func() {
			if sub != nil {
				if err := sub.Unsubscribe(); err != nil {
					log.Printf("unsubscribe %s failed: %v", subject, err)
				}
			}
			close(ch)
		})
		close(ch)
	}

	return ch, cancel, nil
}
