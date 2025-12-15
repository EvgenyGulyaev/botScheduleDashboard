package routes

import (
	"botDashboard/internal/events/producer"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type resPostMessageSend struct {
	Message string `json:"message"`
}

func PostMessageSend(ctx *silverlining.Context, body []byte) {
	var req producer.Message
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	err = req.Publish()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	err = ctx.WriteJSON(http.StatusOK, resPostMessageSend{Message: "Ok"})
	if err != nil {
		log.Print(err)
	}
}
