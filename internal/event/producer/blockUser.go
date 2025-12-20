package producer

import (
	"botDashboard/pkg/broker"
)

type BlockUser struct {
	User    int64  `json:"user"`
	IsBlock bool   `json:"isBlock"`
	Net     string `json:"net"`
}

func (u *BlockUser) Publish() error {
	b := broker.Get()
	return broker.Publish[BlockUser](b.Nc, "user.block", *u)
}
