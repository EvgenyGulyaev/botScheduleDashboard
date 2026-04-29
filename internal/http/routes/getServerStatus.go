package routes

import (
	"botDashboard/internal/system"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetServerStatus(ctx *silverlining.Context) {
	if err := ctx.WriteJSON(http.StatusOK, system.CollectInfo()); err != nil {
		logChatError(err)
	}
}
