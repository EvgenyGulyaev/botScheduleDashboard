package routes

import (
	"botDashboard/internal/http/status"
	"github.com/go-www/silverlining"
	"log"
)

func NotFound(ctx *silverlining.Context) {
	data := map[string]int{"error": status.NOT_FOUND}

	err := ctx.WriteJSON(status.NOT_FOUND, data)
	if err != nil {
		log.Print(err)
	}
}
