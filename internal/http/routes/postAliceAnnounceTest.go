package routes

import (
	"botDashboard/internal/alice"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type postAliceAnnounceTestBody struct {
	Text        string `json:"text"`
	AccountID   string `json:"account_id"`
	HouseholdID string `json:"household_id"`
	RoomID      string `json:"room_id"`
	DeviceID    string `json:"device_id"`
	Voice       string `json:"voice"`
}

func PostAliceAnnounceTest(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}
	if !user.IsAdmin {
		GetError(ctx, &Error{Message: "admin access required", Status: http.StatusForbidden})
		return
	}

	var payload postAliceAnnounceTestBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	payload.Text = strings.TrimSpace(payload.Text)
	payload.AccountID = strings.TrimSpace(payload.AccountID)
	payload.HouseholdID = strings.TrimSpace(payload.HouseholdID)
	payload.RoomID = strings.TrimSpace(payload.RoomID)
	payload.DeviceID = strings.TrimSpace(payload.DeviceID)
	payload.Voice = strings.TrimSpace(payload.Voice)

	if payload.Text == "" {
		GetError(ctx, &Error{Message: "text is required", Status: http.StatusBadRequest})
		return
	}
	if payload.AccountID == "" {
		GetError(ctx, &Error{Message: "account_id is required", Status: http.StatusBadRequest})
		return
	}
	if payload.DeviceID == "" {
		GetError(ctx, &Error{Message: "device_id is required", Status: http.StatusBadRequest})
		return
	}

	client := alice.NewClient()
	if !client.Enabled() {
		GetError(ctx, &Error{Message: "alice service is not configured", Status: http.StatusServiceUnavailable})
		return
	}

	response, err := client.AnnounceScenario(alice.AnnounceRequest{
		AccountID:      payload.AccountID,
		HouseholdID:    payload.HouseholdID,
		RoomID:         payload.RoomID,
		DeviceID:       payload.DeviceID,
		Voice:          payload.Voice,
		InitiatorEmail: user.Email,
		Text:           payload.Text,
	})
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}
