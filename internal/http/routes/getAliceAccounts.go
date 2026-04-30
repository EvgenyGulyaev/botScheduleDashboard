package routes

import (
	"botDashboard/internal/alice"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetAliceAccounts(ctx *silverlining.Context) {
	if _, ok := requireAliceAccess(ctx); !ok {
		return
	}

	client := alice.NewClient()
	if !client.Enabled() {
		GetError(ctx, &Error{Message: "alice service is not configured", Status: http.StatusServiceUnavailable})
		return
	}

	items, err := client.ListAccounts()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]any{"items": items}); err != nil {
		logChatError(err)
	}
}
