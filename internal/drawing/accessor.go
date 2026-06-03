package drawing

import (
	"botDashboard/pkg/singleton"
)

func GetClient() *Client {
	return singleton.GetInstance("drawing-client", func() interface{} {
		cfg, err := LoadConfigFromEnv()
		if err != nil {
			return (*Client)(nil)
		}
		return NewClient(cfg)
	}).(*Client)
}
