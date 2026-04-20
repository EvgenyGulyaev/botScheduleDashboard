package push

import (
	"botDashboard/internal/config"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type ChatNotificationPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

type vapidConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

func configFromEnv() vapidConfig {
	env := config.LoadConfig().Env
	return vapidConfig{
		PublicKey:  strings.TrimSpace(env["CHAT_PUSH_VAPID_PUBLIC_KEY"]),
		PrivateKey: strings.TrimSpace(env["CHAT_PUSH_VAPID_PRIVATE_KEY"]),
		Subject:    strings.TrimSpace(env["CHAT_PUSH_VAPID_SUBJECT"]),
	}
}

func Enabled() bool {
	cfg := configFromEnv()
	return cfg.PublicKey != "" && cfg.PrivateKey != "" && cfg.Subject != ""
}

func PublicKey() string {
	return configFromEnv().PublicKey
}

func SendChatMessageNotification(recipient model.UserData, subscriptions []model.PushSubscription, payload ChatNotificationPayload) error {
	if !Enabled() || !recipient.NotificationSettings.PushEnabled || len(subscriptions) == 0 {
		return nil
	}

	cfg := configFromEnv()
	repo := store.GetUserRepository()
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	for _, subscription := range subscriptions {
		resp, err := webpush.SendNotification(rawPayload, &webpush.Subscription{
			Endpoint: subscription.Endpoint,
			Keys: webpush.Keys{
				P256dh: subscription.Keys.P256DH,
				Auth:   subscription.Keys.Auth,
			},
		}, &webpush.Options{
			Subscriber:      cfg.Subject,
			VAPIDPublicKey:  cfg.PublicKey,
			VAPIDPrivateKey: cfg.PrivateKey,
			TTL:             60,
		})
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			_ = repo.DeletePushSubscription(recipient.Email, subscription.Endpoint)
		}
	}

	return nil
}

func NotifyChatMembersAboutMessage(conversation model.ChatConversation, members []model.ChatMember, message model.ChatMessage) {
	if !Enabled() {
		return
	}

	repo := store.GetUserRepository()
	for _, member := range members {
		if strings.TrimSpace(member.Email) == "" || member.Email == message.SenderEmail {
			continue
		}

		recipient, err := repo.FindUserByEmail(member.Email)
		if err != nil || !recipient.NotificationSettings.PushEnabled {
			continue
		}

		subscriptions, err := repo.ListPushSubscriptions(member.Email)
		if err != nil || len(subscriptions) == 0 {
			continue
		}

		payload := BuildIncomingMessageNotification(conversation.Title, message)
		go func(user model.UserData, subs []model.PushSubscription, pushPayload ChatNotificationPayload) {
			_ = SendChatMessageNotification(user, subs, pushPayload)
		}(recipient, subscriptions, payload)
	}
}

func BuildIncomingMessageNotification(conversationTitle string, message model.ChatMessage) ChatNotificationPayload {
	sender := strings.TrimSpace(message.SenderLogin)
	if sender == "" {
		sender = strings.TrimSpace(message.SenderEmail)
	}
	if sender == "" {
		sender = "Пользователь"
	}

	title := strings.TrimSpace(conversationTitle)
	if title == "" {
		title = fmt.Sprintf("Новое сообщение от %s", sender)
	}

	body := strings.TrimSpace(message.Text)
	if body == "" {
		switch message.Type {
		case "audio":
			body = "Голосовое сообщение"
		case "image":
			body = "Изображение"
		case "call":
			body = "Начался звонок"
		default:
			body = "Новое сообщение"
		}
	}

	return ChatNotificationPayload{
		Title: title,
		Body:  body,
		URL:   "/chat",
	}
}
