package routes

import (
	"botDashboard/internal/mail"
	"botDashboard/internal/store"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type postForgotPasswordBody struct {
	Email string `json:"email"`
}

type forgotPasswordResponse struct {
	Message string `json:"message"`
}

var sendMail = mail.Send

func SetSendMailForTest(fn func(string, string, string) error) func() {
	previous := sendMail
	sendMail = fn
	return func() {
		sendMail = previous
	}
}

func PostForgotPassword(ctx *silverlining.Context, body []byte) {
	var payload postForgotPasswordBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	email := strings.TrimSpace(payload.Email)
	if email == "" {
		GetError(ctx, &Error{Message: "email is required", Status: http.StatusBadRequest})
		return
	}

	repo := store.GetUserRepository()
	if _, err := repo.FindUserByEmail(email); err == nil {
		if token, tokenErr := repo.CreatePasswordResetToken(email, passwordResetTTL()); tokenErr == nil {
			resetURL := fmt.Sprintf("%s/reset-password?token=%s", appBaseURL(), token)
			subject := "Сброс пароля"
			bodyText := "Чтобы установить новый пароль, открой ссылку:\n\n" + resetURL + "\n\nЕсли это были не вы, просто проигнорируйте письмо."
			if mail.Enabled() {
				if err := sendMail(email, subject, bodyText); err != nil {
					logChatError(err)
				}
			}
		} else {
			logChatError(tokenErr)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, forgotPasswordResponse{
		Message: "Если такой аккаунт существует, мы отправили письмо со ссылкой для сброса пароля.",
	}); err != nil {
		logChatError(err)
	}
}
