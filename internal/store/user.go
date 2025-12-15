package store

import (
	"botDashboard/pkg/db"
	"botDashboard/pkg/singleton"
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

type User struct {
	bucket *[]byte
	db     *bolt.DB
}

func GetUser() *User {
	return singleton.GetInstance("user", func() interface{} {
		return initUserModel()
	}).(*User)
}

func initUserModel() *User {
	return &User{
		bucket: &db.UserBucket,
		db:     db.Init().DB,
	}
}

type UserData struct {
	Login          string
	Email          string
	HashedPassword []byte
	IsAdmin        bool
}

func encodeUser(user UserData) ([]byte, error) {
	return json.Marshal(user)
}

func decodeUser(data []byte) (UserData, error) {
	var user UserData
	err := json.Unmarshal(data, &user)
	return user, err
}
