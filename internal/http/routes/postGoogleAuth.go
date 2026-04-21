package routes

import (
	gauth "botDashboard/internal/auth"
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type postGoogleAuthBody struct {
	IDToken string `json:"id_token"`
}

type responsePostGoogleAuth struct {
	Token string `json:"token"`
	model.UserData
}

var verifyGoogleIDToken = gauth.VerifyGoogleIDToken

type GoogleIdentityShim = gauth.GoogleIdentity

func SetVerifyGoogleIDTokenForTest(fn func(string) (gauth.GoogleIdentity, error)) func() {
	previous := verifyGoogleIDToken
	verifyGoogleIDToken = fn
	return func() {
		verifyGoogleIDToken = previous
	}
}

func PostGoogleAuth(ctx *silverlining.Context, body []byte) {
	var payload postGoogleAuthBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}

	identity, err := verifyGoogleIDToken(payload.IDToken)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	user, err := upsertGoogleUser(identity.Email, identity.Name)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	token, err := middleware.GetJwt().CreateToken(user.Email, user.Login)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, responsePostGoogleAuth{Token: token, UserData: user}); err != nil {
		logChatError(err)
	}
}
