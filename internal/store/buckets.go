package store

import (
	"botDashboard/pkg/db"
	"log"
)

var (
	UserBucket                  = []byte("Users")
	SocialBucket                = []byte("SocialUsers")
	ChatConversationsBucket     = []byte("ChatConversations")
	ChatMembersBucket           = []byte("ChatMembers")
	ChatMessagesBucket          = []byte("ChatMessages")
	ChatUserConversationsBucket = []byte("ChatUserConversations")
)

func InitStore() {
	repo := db.GetRepository()
	err := repo.EnsureBuckets([][]byte{
		UserBucket,
		SocialBucket,
		ChatConversationsBucket,
		ChatMembersBucket,
		ChatMessagesBucket,
		ChatUserConversationsBucket,
	})
	if err != nil {
		log.Println(err)
	}
}
