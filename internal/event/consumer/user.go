package consumer

import (
	"botDashboard/internal/store"
	"log"
)

type User struct {
	Id   int64  `json:"id"`
	Name string `json:"username"`
	Net  string `json:"network"`
	Text string `json:"text"`
}

func HandleUser(u User) {
	r := store.GetSocialUserRepository()
	_, err := r.CreateSocialUser(u.Id, u.Net, u.Name)
	// TODO text кладем текст для отображения в будущем в истории, чтобы общаться при блокировке
	if err != nil {
		log.Println(err)
	}
}
