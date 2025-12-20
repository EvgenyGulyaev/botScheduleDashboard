package model

type SocialMessage struct {
	Id   int64  `json:"id"`
	Chat int64  `json:"chat"`
	Net  string `json:"network"`
	Text string `json:"text"`
}
