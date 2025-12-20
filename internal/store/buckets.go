package store

import (
	"botDashboard/pkg/db"
	"log"
)

var (
	UserBucket   = []byte("Users")
	SocialBucket = []byte("SocialUsers")
)

func InitStore() {
	repo := db.GetRepository()
	err := repo.EnsureBuckets([][]byte{UserBucket, SocialBucket})
	if err != nil {
		log.Println(err)
	}
}
