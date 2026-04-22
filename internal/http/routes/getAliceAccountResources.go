package routes

import (
	"botDashboard/internal/alice"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetAliceAccountResources(ctx *silverlining.Context, accountID string) {
	if _, err := currentChatUser(ctx); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	client := alice.NewClient()
	if !client.Enabled() {
		GetError(ctx, &Error{Message: "alice service is not configured", Status: http.StatusServiceUnavailable})
		return
	}

	resources, err := client.GetAccountResources(accountID)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, resources); err != nil {
		logChatError(err)
	}
}
