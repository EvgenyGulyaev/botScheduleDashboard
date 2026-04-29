package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-www/silverlining"
)

type chatReminderBody struct {
	RemindAt string `json:"remind_at"`
}

type chatReminderDTO struct {
	ID                string    `json:"id"`
	UserEmail         string    `json:"user_email"`
	UserLogin         string    `json:"user_login"`
	ConversationID    string    `json:"conversation_id"`
	ConversationTitle string    `json:"conversation_title"`
	MessageID         string    `json:"message_id"`
	MessageText       string    `json:"message_text"`
	SenderEmail       string    `json:"sender_email"`
	SenderLogin       string    `json:"sender_login"`
	RemindAt          time.Time `json:"remind_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type chatRemindersDTO struct {
	Reminders []chatReminderDTO `json:"reminders"`
}

func GetChatReminders(ctx *silverlining.Context) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	reminders, err := store.GetChatRepository().ListReminders(user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]chatReminderDTO, 0, len(reminders))
	for _, reminder := range reminders {
		response = append(response, chatReminderDTOFromModel(reminder))
	}

	if err := ctx.WriteJSON(http.StatusOK, chatRemindersDTO{Reminders: response}); err != nil {
		logChatError(err)
	}
}

func PostChatReminder(ctx *silverlining.Context, conversationID, messageID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var payload chatReminderBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	remindAt, err := time.Parse(time.RFC3339, payload.RemindAt)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, "remind_at must be RFC3339")
		return
	}

	reminder, err := store.GetChatRepository().CreateReminder(
		user.Email,
		user.Login,
		conversationID,
		messageID,
		remindAt,
	)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, chatReminderDTOFromModel(reminder)); err != nil {
		logChatError(err)
	}
}

func DeleteChatReminder(ctx *silverlining.Context, reminderID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	if err := store.GetChatRepository().DeleteReminder(reminderID, user.Email); err != nil {
		writeChatError(ctx, http.StatusNotFound, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"id": reminderID}); err != nil {
		logChatError(err)
	}
}

func chatReminderDTOFromModel(reminder model.ChatReminder) chatReminderDTO {
	return chatReminderDTO{
		ID:                reminder.ID,
		UserEmail:         reminder.UserEmail,
		UserLogin:         reminder.UserLogin,
		ConversationID:    reminder.ConversationID,
		ConversationTitle: reminder.ConversationTitle,
		MessageID:         reminder.MessageID,
		MessageText:       reminder.MessageText,
		SenderEmail:       reminder.SenderEmail,
		SenderLogin:       reminder.SenderLogin,
		RemindAt:          reminder.RemindAt,
		CreatedAt:         reminder.CreatedAt,
	}
}
