package routes

import (
	"log"

	"github.com/go-www/silverlining"
)

type Error struct {
	Message string
	Status  int
}

func GetError(ctx *silverlining.Context, value *Error) {
	err := ctx.WriteJSON(value.Status, value)
	if err != nil {
		log.Print(err)
	}
}
