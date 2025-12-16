package consumer

import (
	"botDashboard/internal/store"
	"log"
)

type User struct {
	Id   int64  `json:"id"`
	Name string `json:"username"`
	Net  string `json:"network"`
}

func HandleUser(u User) {
	r := store.GetSocialUserRepository()
	_, err := r.CreateSocialUser(u.Id, u.Net, u.Name)
	if err != nil {
		log.Println(err)
	}
}
