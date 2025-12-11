package routes

import (
	"botDashboard/internal/command"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetBotStatus(ctx *silverlining.Context) {
	service, err := ctx.GetQueryParamString("service")
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	c := &command.Status{ServiceName: service}
	text := c.Execute()

	err = ctx.WriteJSON(http.StatusOK, c.Info(text))
	if err != nil {
		log.Print(err)
	}
}
