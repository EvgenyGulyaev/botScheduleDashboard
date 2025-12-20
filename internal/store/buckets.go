package store

import (
	"botDashboard/pkg/db"
	"log"
)

var (
	UserBucket          = []byte("Users")
	SocialBucket        = []byte("SocialUsers")
	SocialMessageBucket = []byte("SocialMessage")
)

func InitStore() {
	repo := db.GetRepository()
	err := repo.EnsureBuckets([][]byte{UserBucket, SocialBucket, SocialMessageBucket})
	if err != nil {
		log.Println(err)
	}
}
