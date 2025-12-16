package producer

import (
	"botDashboard/pkg/broker"
)

type Message struct {
	User    int64  `json:"user"`
	Message string `json:"message"`
	Network string `json:"network"`
}

func (u *Message) Publish() error {
	b := broker.Get()
	return broker.Publish[Message](b.Nc, "message", *u)
}
