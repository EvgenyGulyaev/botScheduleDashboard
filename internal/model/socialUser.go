package model

type SocialUser struct {
	Id   int64  `json:"id"`
	Name string `json:"username"`
	Net  string `json:"network"`
}
