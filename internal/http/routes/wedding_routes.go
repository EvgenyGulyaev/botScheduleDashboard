package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type weddingRSVPsDTO struct {
	Items []model.WeddingRSVP `json:"items"`
}

func GetWeddingPublicSettings(ctx *silverlining.Context) {
	settings, err := store.GetWeddingRepository().GetSettings()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, settings); err != nil {
		logChatError(err)
	}
}

func PostWeddingRSVP(ctx *silverlining.Context, body []byte) {
	var input model.WeddingRSVP
	if err := json.Unmarshal(body, &input); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	item, err := store.GetWeddingRepository().CreateRSVP(input)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		logChatError(err)
	}
}

func GetWeddingRSVPs(ctx *silverlining.Context) {
	if !requireWeddingAccess(ctx) {
		return
	}
	items, err := store.GetWeddingRepository().ListRSVPs()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, weddingRSVPsDTO{Items: items}); err != nil {
		logChatError(err)
	}
}

func DeleteWeddingRSVP(ctx *silverlining.Context, id string) {
	if !requireWeddingAccess(ctx) {
		return
	}
	if id == "" {
		GetError(ctx, &Error{Message: "rsvp id is required", Status: http.StatusBadRequest})
		return
	}
	deleted, err := store.GetWeddingRepository().DeleteRSVP(id)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if !deleted {
		GetError(ctx, &Error{Message: "rsvp not found", Status: http.StatusNotFound})
		return
	}
	ctx.WriteHeader(http.StatusNoContent)
}

func GetWeddingSettings(ctx *silverlining.Context) {
	if !requireWeddingAccess(ctx) {
		return
	}
	settings, err := store.GetWeddingRepository().GetSettings()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, settings); err != nil {
		logChatError(err)
	}
}

func PatchWeddingSettings(ctx *silverlining.Context, body []byte) {
	if !requireWeddingAccess(ctx) {
		return
	}
	var input model.WeddingSettings
	if err := json.Unmarshal(body, &input); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	settings, err := store.GetWeddingRepository().SaveSettings(input)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, settings); err != nil {
		logChatError(err)
	}
}

func requireWeddingAccess(ctx *silverlining.Context) bool {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return false
	}
	if !model.AppAllowed(model.DefaultAppWedding, user.AppPermissions) {
		GetError(ctx, &Error{Message: "wedding access is not allowed for this user", Status: http.StatusForbidden})
		return false
	}
	return true
}
