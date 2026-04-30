package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func userCanUseAlice(user model.UserData) bool {
	return model.AppAllowed(model.DefaultAppAlice, user.AppPermissions)
}

func requireAliceAccess(ctx *silverlining.Context) (model.UserData, bool) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return model.UserData{}, false
	}
	if !userCanUseAlice(user) {
		GetError(ctx, &Error{Message: "alice access is not allowed for this user", Status: http.StatusForbidden})
		return model.UserData{}, false
	}
	return user, true
}

func canSeeChatUser(viewer, target model.UserData) bool {
	if viewer.IsSuperAdmin || viewer.Email == target.Email {
		return true
	}
	return model.ShareVisibilityGroup(viewer.VisibilityGroups, target.VisibilityGroups)
}

func filterVisibleChatUsers(viewer model.UserData, users []model.UserData) []model.UserData {
	if viewer.IsSuperAdmin {
		return users
	}
	result := make([]model.UserData, 0, len(users))
	for _, user := range users {
		if canSeeChatUser(viewer, user) {
			result = append(result, user)
		}
	}
	return result
}

func conversationMembersVisibleForUser(viewer model.UserData, members []model.ChatMember) bool {
	if viewer.IsSuperAdmin {
		return true
	}
	for _, member := range members {
		if member.Email == viewer.Email {
			continue
		}
		target, err := store.GetUserRepository().FindUserByEmail(member.Email)
		if err != nil {
			continue
		}
		if !canSeeChatUser(viewer, target) {
			return false
		}
	}
	return true
}
