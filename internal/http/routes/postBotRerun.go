package routes

import (
	"botDashboard/internal/command"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type bodyPostBotRestart struct {
	Service string `json:"service"`
}

type resBotRestart struct {
	Message string `json:"message"`
}

func PostBotRestart(ctx *silverlining.Context, body []byte) {
	var req bodyPostBotRestart
	err := json.Unmarshal(body, &req)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if req.Service == "" {
		GetError(ctx, &Error{Message: "Service is required", Status: http.StatusBadRequest})
	}

	text := (&command.Restart{ServiceName: req.Service}).Execute()

	err = ctx.WriteJSON(http.StatusOK, resBotRestart{Message: text})
	if err != nil {
		log.Print(err)
	}
}
