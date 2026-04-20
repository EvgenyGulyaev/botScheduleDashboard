package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetChatCall(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	call, err := store.GetChatRepository().GetActiveCall(conversationID)
	if err != nil {
		if err := ctx.WriteJSON(http.StatusOK, nil); err != nil {
			logChatError(err)
		}
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, chatCallDTOFromModel(call)); err != nil {
		logChatError(err)
	}
}
