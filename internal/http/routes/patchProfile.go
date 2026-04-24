package routes

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/http/validator"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type patchProfileBody struct {
	Login            *string `json:"login"`
	Email            *string `json:"email"`
	Password         *string `json:"password"`
	DefaultApp       *string `json:"default_app"`
	AliceAccountID   *string `json:"alice_account_id"`
	AliceHouseholdID *string `json:"alice_household_id"`
	AliceRoomID      *string `json:"alice_room_id"`
	AliceDeviceID    *string `json:"alice_device_id"`
	AliceScenarioID  *string `json:"alice_scenario_id"`
	AliceDisabled    *bool   `json:"alice_disabled"`
	PushEnabled      *bool   `json:"push_enabled"`
	SoundEnabled     *bool   `json:"sound_enabled"`
	ToastEnabled     *bool   `json:"toast_enabled"`
}

func PatchProfile(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	var payload patchProfileBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	updated := user
	prevEmail := user.Email
	refreshSession := false

	if payload.Login != nil {
		login := strings.TrimSpace(*payload.Login)
		if login == "" {
			GetError(ctx, &Error{Message: "login is required", Status: http.StatusBadRequest})
			return
		}
		if login != updated.Login {
			updated.Login = login
			refreshSession = true
		}
	}

	if payload.Email != nil {
		email := strings.TrimSpace(*payload.Email)
		if email == "" {
			GetError(ctx, &Error{Message: "email is required", Status: http.StatusBadRequest})
			return
		}
		emailValidator := validator.UserEmailValidator{Email: email}
		if !emailValidator.Validate() {
			GetError(ctx, &Error{Message: "Email is invalid", Status: http.StatusBadRequest})
			return
		}
		if email != updated.Email {
			if _, err := store.GetUserRepository().FindUserByEmail(email); err == nil {
				GetError(ctx, &Error{Message: "user with this email already exists", Status: http.StatusBadRequest})
				return
			}
			updated.Email = email
			refreshSession = true
		}
	}

	if payload.Password != nil {
		password := strings.TrimSpace(*payload.Password)
		if password == "" {
			GetError(ctx, &Error{Message: "password is required", Status: http.StatusBadRequest})
			return
		}
		hash, err := store.GetUserRepository().HashPassword(password)
		if err != nil {
			GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
			return
		}
		updated.HashedPassword = hash
	}

	if payload.DefaultApp != nil {
		updated.DefaultApp = model.NormalizeDefaultApp(strings.TrimSpace(*payload.DefaultApp))
	}
	if payload.AliceAccountID != nil {
		updated.AliceSettings.AccountID = strings.TrimSpace(*payload.AliceAccountID)
	}
	if payload.AliceHouseholdID != nil {
		updated.AliceSettings.HouseholdID = strings.TrimSpace(*payload.AliceHouseholdID)
	}
	if payload.AliceRoomID != nil {
		updated.AliceSettings.RoomID = strings.TrimSpace(*payload.AliceRoomID)
	}
	if payload.AliceDeviceID != nil {
		updated.AliceSettings.DeviceID = strings.TrimSpace(*payload.AliceDeviceID)
	}
	if payload.AliceScenarioID != nil {
		updated.AliceSettings.ScenarioID = strings.TrimSpace(*payload.AliceScenarioID)
	}
	if payload.AliceDisabled != nil {
		updated.AliceSettings.Disabled = *payload.AliceDisabled
	}

	if payload.PushEnabled != nil {
		updated.NotificationSettings.PushEnabled = *payload.PushEnabled
		updated.NotificationSettings.Configured = true
	}
	if payload.SoundEnabled != nil {
		updated.NotificationSettings.SoundEnabled = *payload.SoundEnabled
		updated.NotificationSettings.Configured = true
	}
	if payload.ToastEnabled != nil {
		updated.NotificationSettings.ToastEnabled = *payload.ToastEnabled
		updated.NotificationSettings.Configured = true
	}

	if err := store.GetUserRepository().UpdateUser(updated, prevEmail); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	if refreshSession {
		if err := middleware.GetJwt().RefreshSession(ctx, updated.Email, updated.Login); err != nil {
			GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
			return
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, profileDTOFromUser(updated)); err != nil {
		logChatError(err)
	}
}
