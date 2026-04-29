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
	ChatFavoritesBucket         = []byte("ChatFavorites")
	ChatUserConversationsBucket = []byte("ChatUserConversations")
	UserPushSubscriptionsBucket = []byte("UserPushSubscriptions")
	PasswordResetTokensBucket   = []byte("PasswordResetTokens")
	AuditBucket                 = []byte("AuditLog")
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
		ChatFavoritesBucket,
		ChatUserConversationsBucket,
		UserPushSubscriptionsBucket,
		PasswordResetTokensBucket,
		AuditBucket,
	})
	if err != nil {
		log.Println(err)
	}
}
