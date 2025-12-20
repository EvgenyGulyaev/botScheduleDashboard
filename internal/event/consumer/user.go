package consumer

import (
	"botDashboard/internal/store"
	"log"
)

type User struct {
	Id    int64  `json:"id"`
	Name  string `json:"username"`
	Net   string `json:"network"`
	Text  string `json:"text"`
	MesId int    `json:"mes_id"`
}

func HandleUser(u User) {
	r := store.GetSocialUserRepository()
	// Добавляется история сообщений
	user, err := r.FindByID(u.Id, u.Net)
	if err == nil {
		if user.Messages == nil {
			user.Messages = make(map[int]string)
		}
		user.Messages[u.MesId] = u.Text
		err = r.UpdateUserMessages(user)
		if err != nil {
			log.Println(err)
		}
		return
	}

	// Добавляется пользователь
	_, err = r.CreateSocialUser(u.Id, u.Net, u.Name, u.MesId, u.Text)
	if err != nil {
		log.Println(err)
	}
}
