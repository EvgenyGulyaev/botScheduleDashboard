package routes

import (
	"botDashboard/internal/alice"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type postAliceCleanupScenariosBody struct {
	AccountID string `json:"account_id"`
	DeviceID  string `json:"device_id"`
}

func PostAliceCleanupScenarios(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}
	if !user.IsAdmin {
		GetError(ctx, &Error{Message: "admin access required", Status: http.StatusForbidden})
		return
	}

	var payload postAliceCleanupScenariosBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	payload.AccountID = strings.TrimSpace(payload.AccountID)
	payload.DeviceID = strings.TrimSpace(payload.DeviceID)

	if payload.AccountID == "" {
		GetError(ctx, &Error{Message: "account_id is required", Status: http.StatusBadRequest})
		return
	}

	client := alice.NewClient()
	if !client.Enabled() {
		GetError(ctx, &Error{Message: "alice service is not configured", Status: http.StatusServiceUnavailable})
		return
	}

	response, err := client.CleanupScenarios(payload.AccountID, alice.CleanupScenariosRequest{
		DeviceID: payload.DeviceID,
	})
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}
