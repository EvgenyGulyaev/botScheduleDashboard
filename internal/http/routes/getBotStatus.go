package routes

import (
	"botDashboard/internal/command"
	"log"
	"net/http"
	"strconv"

	"github.com/go-www/silverlining"
)

func GetBotStatus(ctx *silverlining.Context) {
	service, err := ctx.GetQueryParamString("service")
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	logLines := 0
	if rawLines, err := ctx.GetQueryParamString("lines"); err == nil && rawLines != "" {
		if value, parseErr := strconv.Atoi(rawLines); parseErr == nil {
			logLines = value
		}
	}

	c := &command.Status{ServiceName: service, LogLines: logLines}

	err = ctx.WriteJSON(http.StatusOK, c.Details())
	if err != nil {
		log.Print(err)
	}
}
