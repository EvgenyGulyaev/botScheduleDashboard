package routes

import (
	"botDashboard/internal/http/validator"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type adminUserDTO struct {
	Login            string   `json:"login"`
	Email            string   `json:"email"`
	IsAdmin          bool     `json:"is_admin"`
	IsSuperAdmin     bool     `json:"is_super_admin"`
	DefaultApp       string   `json:"default_app"`
	AppPermissions   []string `json:"app_permissions"`
	VisibilityGroups []string `json:"visibility_groups"`
}

type adminUsersDTO struct {
	Items []adminUserDTO `json:"items"`
}

type adminUserBody struct {
	Login            *string  `json:"login"`
	Email            *string  `json:"email"`
	Password         *string  `json:"password"`
	IsAdmin          *bool    `json:"is_admin"`
	IsSuperAdmin     *bool    `json:"is_super_admin"`
	DefaultApp       *string  `json:"default_app"`
	AppPermissions   []string `json:"app_permissions"`
	VisibilityGroups []string `json:"visibility_groups"`
}

func GetAdminUsers(ctx *silverlining.Context) {
	users, err := store.GetUserRepository().ListAll()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	items := make([]adminUserDTO, 0, len(users))
	for _, user := range users {
		items = append(items, adminUserDTOFromUser(user))
	}
	if err := ctx.WriteJSON(http.StatusOK, adminUsersDTO{Items: items}); err != nil {
		logChatError(err)
	}
}

func PostAdminUser(ctx *silverlining.Context, body []byte) {
	var payload adminUserBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	login := trimStringPtr(payload.Login)
	email := trimStringPtr(payload.Email)
	password := trimStringPtr(payload.Password)
	if login == "" || email == "" || password == "" {
		GetError(ctx, &Error{Message: "login, email and password are required", Status: http.StatusBadRequest})
		return
	}
	if !(validator.UserEmailValidator{Email: email}).Validate() {
		GetError(ctx, &Error{Message: "Email is invalid", Status: http.StatusBadRequest})
		return
	}

	repo := store.GetUserRepository()
	user, err := repo.CreateUser(login, email, password)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	applyAdminUserPayload(&user, payload)
	if err := repo.UpdateUser(user, ""); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	user, _ = repo.FindUserByEmail(user.Email)
	recordAdminAudit(ctx, model.AuditActionAdminUserCreate, user.Email, "Создан пользователь "+user.Email, map[string]string{
		"login": user.Login,
	})
	if err := ctx.WriteJSON(http.StatusOK, adminUserDTOFromUser(user)); err != nil {
		logChatError(err)
	}
}

func PatchAdminUser(ctx *silverlining.Context, email string, body []byte) {
	var payload adminUserBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	repo := store.GetUserRepository()
	user, err := repo.FindUserByEmail(email)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusNotFound})
		return
	}
	prevEmail := user.Email
	if payload.Password != nil {
		password := trimStringPtr(payload.Password)
		if password == "" {
			GetError(ctx, &Error{Message: "password is required", Status: http.StatusBadRequest})
			return
		}
		hash, err := repo.HashPassword(password)
		if err != nil {
			GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
			return
		}
		user.HashedPassword = hash
	}
	applyAdminUserPayload(&user, payload)
	if user.Login == "" || user.Email == "" {
		GetError(ctx, &Error{Message: "login and email are required", Status: http.StatusBadRequest})
		return
	}
	if !(validator.UserEmailValidator{Email: user.Email}).Validate() {
		GetError(ctx, &Error{Message: "Email is invalid", Status: http.StatusBadRequest})
		return
	}
	if user.Email != prevEmail {
		if _, err := repo.FindUserByEmail(user.Email); err == nil {
			GetError(ctx, &Error{Message: "user with this email already exists", Status: http.StatusBadRequest})
			return
		}
	}

	if err := repo.UpdateUser(user, prevEmail); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	user, _ = repo.FindUserByEmail(user.Email)
	recordAdminAudit(ctx, model.AuditActionAdminUserUpdate, user.Email, "Обновлён пользователь "+user.Email, map[string]string{
		"previous_email": prevEmail,
		"login":          user.Login,
	})
	if err := ctx.WriteJSON(http.StatusOK, adminUserDTOFromUser(user)); err != nil {
		logChatError(err)
	}
}

func DeleteAdminUser(ctx *silverlining.Context, email string) {
	current, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}
	if current.Email == email {
		GetError(ctx, &Error{Message: "cannot delete current user", Status: http.StatusBadRequest})
		return
	}
	if err := store.GetUserRepository().DeleteUser(email); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	recordAdminAudit(ctx, model.AuditActionAdminUserDelete, email, "Удалён пользователь "+email, nil)
	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}
}

func adminUserDTOFromUser(user model.UserData) adminUserDTO {
	return adminUserDTO{
		Login:            user.Login,
		Email:            user.Email,
		IsAdmin:          user.IsAdmin,
		IsSuperAdmin:     user.IsSuperAdmin,
		DefaultApp:       user.DefaultApp,
		AppPermissions:   user.AppPermissions,
		VisibilityGroups: user.VisibilityGroups,
	}
}

func applyAdminUserPayload(user *model.UserData, payload adminUserBody) {
	if payload.Login != nil {
		user.Login = trimStringPtr(payload.Login)
	}
	if payload.Email != nil {
		user.Email = trimStringPtr(payload.Email)
	}
	if payload.IsAdmin != nil {
		user.IsAdmin = *payload.IsAdmin
	}
	if payload.IsSuperAdmin != nil {
		user.IsSuperAdmin = *payload.IsSuperAdmin
	}
	if user.IsSuperAdmin {
		user.IsAdmin = true
	}
	if payload.AppPermissions != nil {
		user.AppPermissions = model.NormalizeAppPermissions(payload.AppPermissions, user.IsAdmin, user.IsSuperAdmin)
	}
	if payload.VisibilityGroups != nil {
		user.VisibilityGroups = model.NormalizeVisibilityGroups(payload.VisibilityGroups)
	}
	if payload.DefaultApp != nil {
		user.DefaultApp = model.NormalizeDefaultAppForPermissions(strings.TrimSpace(*payload.DefaultApp), user.AppPermissions)
	}
}

func trimStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
