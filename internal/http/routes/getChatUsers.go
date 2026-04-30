package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetChatUsers(ctx *silverlining.Context) {
	currentUser, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	users, err := store.GetUserRepository().ListAll()
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, chatUserDTOs(filterVisibleChatUsers(currentUser, users))); err != nil {
		logChatError(err)
	}
}
