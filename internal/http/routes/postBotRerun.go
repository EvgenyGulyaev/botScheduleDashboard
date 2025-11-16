package routes

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type bodyPostBotRerun struct {
	BotService string `json:"bot"`
}

func PostBotRerun(ctx *silverlining.Context, body []byte) {
	var req bodyPostBotRerun
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	err = ctx.WriteJSON(http.StatusOK, "Бот успешно перезапущен")
	if err != nil {
		log.Print(err)
	}
}
