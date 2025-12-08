package routes

import (
	"botDashboard/pkg/broker"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-www/silverlining"
)

type bodyPostMessageSend struct {
	User    string `json:"user"`
	Message string `json:"message"`
	Network string `json:"network"`
}

type resPostMessageSend struct {
	Message string `json:"message"`
}

func PostMessageSend(ctx *silverlining.Context, body []byte) {
	var req bodyPostMessageSend
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	b, err := broker.NewNatsBroker(os.Getenv("NATS_URL"))
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	err = broker.Publish[bodyPostMessageSend](b.Nc, "message", req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	defer b.Close()

	err = ctx.WriteJSON(http.StatusOK, resPostMessageSend{Message: "Ok"})
	if err != nil {
		log.Print(err)
	}
}
