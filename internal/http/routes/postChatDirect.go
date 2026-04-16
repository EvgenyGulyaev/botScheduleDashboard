package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatDirect(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req chatDirectBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email == "" {
		writeChatError(ctx, http.StatusBadRequest, "email is required")
		return
	}

	target, err := store.GetUserRepository().FindUserByEmail(req.Email)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: user.Email,
		Login: user.Login,
	}, model.ChatMember{
		Email: target.Email,
		Login: target.Login,
	})
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	view, err := conversationView(ctx, conv.ID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, view); err != nil {
		logChatError(err)
	}
}
