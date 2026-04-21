package routes

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type postResetPasswordBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type responsePostResetPassword struct {
	Token string `json:"token"`
	model.UserData
}

func PostResetPassword(ctx *silverlining.Context, body []byte) {
	var payload postResetPasswordBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	tokenValue := strings.TrimSpace(payload.Token)
	password := strings.TrimSpace(payload.Password)
	if tokenValue == "" || password == "" {
		GetError(ctx, &Error{Message: "token and password are required", Status: http.StatusBadRequest})
		return
	}

	repo := store.GetUserRepository()
	record, err := repo.ConsumePasswordResetToken(tokenValue)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	user, err := repo.FindUserByEmail(record.Email)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	hash, err := repo.HashPassword(password)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	user.HashedPassword = hash
	if err := repo.UpdateUser(user, user.Email); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	token, err := middleware.GetJwt().CreateToken(user.Email, user.Login)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, responsePostResetPassword{Token: token, UserData: user}); err != nil {
		logChatError(err)
	}
}
