package routes

import (
	"botDashboard/internal/command"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type resBotStatus struct {
	Message string `json:"message"`
}

func GetBotStatus(ctx *silverlining.Context) {
	service, err := ctx.GetQueryParamString("service")
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	text := (&command.Status{ServiceName: service}).Execute()

	err = ctx.WriteJSON(http.StatusOK, resBotStatus{Message: text})
	if err != nil {
		log.Print(err)
	}
}
