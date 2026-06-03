package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/push"
	"botDashboard/internal/store"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

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
	if err := ctx.WriteJSON(http.StatusOK, settings.Public()); err != nil {
		logChatError(err)
	}
}

func PostWeddingAccessVerify(ctx *silverlining.Context, body []byte) {
	var input model.WeddingAccessVerifyInput
	if err := json.Unmarshal(body, &input); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	result, err := store.GetWeddingRepository().VerifyAccessCode(weddingClientIP(ctx), input.Code, time.Now().UTC())
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	status := http.StatusOK
	if result.Locked {
		status = http.StatusTooManyRequests
	} else if !result.OK {
		status = http.StatusUnauthorized
	}
	if err := ctx.WriteJSON(status, result); err != nil {
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

	sendWeddingRSVPNotifications(item)
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

func PatchWeddingRSVP(ctx *silverlining.Context, id string, body []byte) {
	if !requireWeddingAccess(ctx) {
		return
	}
	if id == "" {
		GetError(ctx, &Error{Message: "rsvp id is required", Status: http.StatusBadRequest})
		return
	}
	var input model.WeddingRSVP
	if err := json.Unmarshal(body, &input); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	item, ok, err := store.GetWeddingRepository().UpdateRSVP(id, input)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if !ok {
		GetError(ctx, &Error{Message: "rsvp not found", Status: http.StatusNotFound})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		logChatError(err)
	}
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

func sendWeddingRSVPNotifications(rsvp model.WeddingRSVP) {
	users, err := store.GetUserRepository().ListAll()
	if err != nil {
		logChatError(fmt.Errorf("list users for wedding notifications: %w", err))
		return
	}

	attendance := "Буду"
	if rsvp.Attendance == model.WeddingAttendanceNotAttending {
		attendance = "Не Буду"
	}
	text := fmt.Sprintf("%s - %s", rsvp.FullName, attendance)

	recipients := make([]model.ChatMember, 0, len(users))
	for _, user := range users {
		if model.AppAllowed(model.DefaultAppWedding, user.AppPermissions) {
			recipients = append(recipients, model.ChatMember{
				Email: user.Email,
				Login: user.Login,
			})
		}
	}
	if len(recipients) == 0 {
		return
	}

	results, err := store.GetChatRepository().AddSystemNotificationsBatch(recipients, text)
	if err != nil {
		logChatError(fmt.Errorf("send wedding notifications batch: %w", err))
		return
	}
	for _, result := range results {
		if err := producer.PublishChatMessagePersistedEvent(event.ChatMessagePersistedEvent{
			Conversation: result.Conversation,
			Members:      result.Members,
			Message:      result.Message,
		}); err != nil {
			logChatError(err)
		}
		push.NotifyChatMembersAboutMessage(result.Conversation, result.Members, result.Message)
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

func weddingClientIP(ctx *silverlining.Context) string {
	if value, ok := ctx.RequestHeaders().Get("X-Forwarded-For"); ok {
		parts := strings.Split(value, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if value, ok := ctx.RequestHeaders().Get("X-Real-IP"); ok {
		if ip := strings.TrimSpace(value); ip != "" {
			return ip
		}
	}
	remote := strings.TrimSpace(ctx.RemoteAddr().String())
	if host, _, err := net.SplitHostPort(remote); err == nil && host != "" {
		return host
	}
	return remote
}
