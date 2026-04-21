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
	PasswordResetTokensBucket   = []byte("PasswordResetTokens")
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
		PasswordResetTokensBucket,
	})
	if err != nil {
		log.Println(err)
	}
}
