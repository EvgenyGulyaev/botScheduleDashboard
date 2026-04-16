package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatGroup(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req chatGroupBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title == "" {
		writeChatError(ctx, http.StatusBadRequest, "title is required")
		return
	}

	emails := uniqueEmails(req.MemberEmails)
	members := make([]model.ChatMember, 0, len(emails)+1)
	members = append(members, model.ChatMember{Email: user.Email, Login: user.Login})
	for _, email := range emails {
		member, err := store.GetUserRepository().FindUserByEmail(email)
		if err != nil {
			writeChatError(ctx, http.StatusBadRequest, err.Error())
			return
		}
		members = append(members, model.ChatMember{
			Email: member.Email,
			Login: member.Login,
		})
	}

	conv, err := store.GetChatRepository().CreateGroupConversation(req.Title, members)
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
