package chat

import (
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func (c *Client) readLoop() {
	defer func() {
		c.hub.Unregister(c)
		c.Close()
	}()

	for {
		raw, err := wsutil.ReadClientText(c.conn)
		if err != nil {
			return
		}
		c.handleIncoming(raw)
	}
}

func (c *Client) writeLoop() {
	defer c.Close()
	for msg := range c.send {
		if err := wsutil.WriteServerMessage(c.conn, ws.OpText, msg); err != nil {
			return
		}
	}
}
