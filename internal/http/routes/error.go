package routes

import (
	"github.com/go-www/silverlining"
	"log"
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
