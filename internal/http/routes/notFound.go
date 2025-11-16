package routes

import (
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

func NotFound(ctx *silverlining.Context) {
	data := map[string]int{"error": http.StatusNotFound}

	err := ctx.WriteJSON(http.StatusNotFound, data)
	if err != nil {
		log.Print(err)
	}
}
