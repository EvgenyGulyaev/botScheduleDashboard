package consumer

import "log"

type User struct {
	Id   int64  `json:"id"`
	Name string `json:"username"`
	Net  string `json:"network"`
}

func HandleUser(u User) {
	log.Println(u)
	// TODO HANDLE
}
