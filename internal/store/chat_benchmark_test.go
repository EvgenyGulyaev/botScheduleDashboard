package store

import (
	"botDashboard/internal/model"
	"fmt"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func BenchmarkDeleteMessage1000(b *testing.B) {
	repo := newChatRepo(b)
	conversation, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		b.Fatalf("create conversation: %v", err)
	}

	var target model.ChatMessage
	err = repo.repo.Update(func(tx *bolt.Tx) error {
		for i := range 1000 {
			createdAt := time.Unix(int64(i), 0).UTC()
			message := model.ChatMessage{
				ID:             fmt.Sprintf("%06d", i),
				ConversationID: conversation.ID,
				SenderEmail:    "alice@example.com",
				SenderLogin:    "alice",
				Type:           "text",
				Text:           "benchmark message",
				CreatedAt:      createdAt,
				UpdatedAt:      createdAt,
			}
			if i > 0 && i <= 10 {
				message.ReplyToMessageID = "000000"
			}
			if i == 0 {
				target = message
			}
			if err := saveMessage(tx, message); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatalf("seed messages: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		err := repo.repo.Update(func(tx *bolt.Tx) error {
			member, err := loadConversationMember(tx, conversation.ID, "bob@example.com")
			if err != nil {
				return err
			}
			member.LastReadMessageID = target.ID
			if err := saveMember(tx, member); err != nil {
				return err
			}
			return saveMessage(tx, target)
		})
		if err != nil {
			b.Fatalf("restore fixture: %v", err)
		}
		b.StartTimer()

		if _, err := repo.DeleteMessage(conversation.ID, target.ID, "alice@example.com"); err != nil {
			b.Fatalf("delete message: %v", err)
		}
	}
}
