package routes

import (
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type deleteProfilePushSubscriptionBody struct {
	Endpoint string `json:"endpoint"`
}

func DeleteProfilePushSubscriptions(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	var payload deleteProfilePushSubscriptionBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
			return
		}
	}

	if err := store.GetUserRepository().DeletePushSubscription(user.Email, strings.TrimSpace(payload.Endpoint)); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}
}
