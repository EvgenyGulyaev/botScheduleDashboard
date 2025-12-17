package command

import (
	"botDashboard/internal/event/producer"
	"os"
	"strconv"
)

type Initial struct {
}

func (r *Initial) Execute() string {
	user, err := strconv.ParseInt(os.Getenv("CHAT_FOR_BUILD"), 10, 64)
	if err != nil {
		return err.Error()
	}
	message := &producer.Message{
		User:    user,
		Message: "Релиз прошел успешно",
		Network: "tg",
	}
	err = message.Publish()
	if err != nil {
		return err.Error()
	}
	return "Релиз прошел успешно"
}
