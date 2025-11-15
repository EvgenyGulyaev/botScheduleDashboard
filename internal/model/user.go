package model

import (
	"botDashboard/pkg/db"
	"botDashboard/pkg/singleton"
	"encoding/json"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	bucket *[]byte
	db     *bolt.DB
}

func GetUser() *User {
	return singleton.GetInstance("user", func() interface{} {
		return initModel()
	}).(*User)

}

func initModel() *User {
	return &User{
		bucket: &db.UserBucket,
		db:     db.Init().DB,
	}
}

type UserData struct {
	Login          string
	Email          string
	HashedPassword []byte
}

func (u *User) CreateUser(login, email, password string) (userData UserData, err error) {
	// Хэшируем пароль
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return
	}

	user := UserData{
		Login:          login,
		Email:          email,
		HashedPassword: hash,
	}

	err = u.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(*u.bucket)
		if b.Get([]byte(email)) != nil {
			return fmt.Errorf("user with email %s already exists", email)
		}
		encoded, err := encodeUser(user)
		if err != nil {
			return err
		}
		return b.Put([]byte(email), encoded)
	})
	if err != nil {
		return
	}
	return u.FindUserByEmail(email)
}

func (u *User) FindUserByEmail(email string) (UserData, error) {
	var user UserData
	err := u.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(*u.bucket)
		data := b.Get([]byte(email))
		if data == nil {
			return fmt.Errorf("user not found")
		}
		var err error
		user, err = decodeUser(data)
		return err
	})
	return user, err
}

func encodeUser(user UserData) ([]byte, error) {
	return json.Marshal(user)
}

func decodeUser(data []byte) (UserData, error) {
	var user UserData
	err := json.Unmarshal(data, &user)
	return user, err
}
