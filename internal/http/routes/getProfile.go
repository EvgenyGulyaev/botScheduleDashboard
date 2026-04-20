package routes

import (
	"net/http"

	"github.com/go-www/silverlining"
)

func GetProfile(ctx *silverlining.Context) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, profileDTOFromUser(user)); err != nil {
		logChatError(err)
	}
}
