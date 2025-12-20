package routes

import (
	"botDashboard/internal/event/producer"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type resPostUserBlock struct {
	Message string `json:"message"`
}

func PostUserBlock(ctx *silverlining.Context, body []byte) {
	var req producer.BlockUser
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

	err = ctx.WriteJSON(http.StatusOK, resPostUserBlock{Message: "Ok"})
	if err != nil {
		log.Print(err)
	}
}
