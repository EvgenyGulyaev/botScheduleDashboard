package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type profilePushSubscriptionBody struct {
	Endpoint  string `json:"endpoint"`
	UserAgent string `json:"user_agent"`
	Keys      struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func PostProfilePushSubscriptions(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	var payload profilePushSubscriptionBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	subscription := model.PushSubscription{
		Endpoint:  strings.TrimSpace(payload.Endpoint),
		UserAgent: strings.TrimSpace(payload.UserAgent),
		Keys: model.PushSubscriptionKeys{
			P256DH: strings.TrimSpace(payload.Keys.P256DH),
			Auth:   strings.TrimSpace(payload.Keys.Auth),
		},
	}

	if err := store.GetUserRepository().SavePushSubscription(user.Email, subscription); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}
}
