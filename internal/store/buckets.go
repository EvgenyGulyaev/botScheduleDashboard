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
	ChatCallsBucket             = []byte("ChatCalls")
	ChatReactionsBucket         = []byte("ChatReactions")
	ChatUserConversationsBucket = []byte("ChatUserConversations")
	UserPushSubscriptionsBucket = []byte("UserPushSubscriptions")
)

func InitStore() {
	repo := db.GetRepository()
	err := repo.EnsureBuckets([][]byte{
		UserBucket,
		SocialBucket,
		ChatConversationsBucket,
		ChatMembersBucket,
		ChatMessagesBucket,
		ChatCallsBucket,
		ChatReactionsBucket,
		ChatUserConversationsBucket,
		UserPushSubscriptionsBucket,
	})
	if err != nil {
		log.Println(err)
	}
}
