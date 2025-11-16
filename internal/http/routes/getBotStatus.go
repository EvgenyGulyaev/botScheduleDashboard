package routes

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type paramsGetBotStatus struct {
	BotService string `json:"bot"`
}

func GetBotStatus(ctx *silverlining.Context, params []byte) {
	var req paramsGetBotStatus
	err := json.Unmarshal(params, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	err = ctx.WriteJSON(http.StatusOK, "Бот имеет такой-то статус")
	if err != nil {
		log.Print(err)
	}
}
