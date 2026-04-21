package routes

import (
	"botDashboard/internal/auth"
	"net/http"

	"github.com/go-www/silverlining"
)

type googleAuthConfigDTO struct {
	Enabled  bool   `json:"enabled"`
	ClientID string `json:"client_id"`
}

func GetGoogleAuthConfig(ctx *silverlining.Context) {
	if err := ctx.WriteJSON(http.StatusOK, googleAuthConfigDTO{
		Enabled:  auth.GoogleEnabled(),
		ClientID: auth.GoogleClientID(),
	}); err != nil {
		logChatError(err)
	}
}
