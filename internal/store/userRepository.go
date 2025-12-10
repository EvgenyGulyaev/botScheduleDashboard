package store

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

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
		IsAdmin:        false,
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

func (u *User) UpdateUser(userData UserData, prevEmail string) (err error) {
	// Если юзер менял емейл
	if userData.Email != prevEmail && prevEmail != "" {
		err = u.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(*u.bucket)
			if err := b.Delete([]byte(prevEmail)); err != nil {
				return err
			}
			return u.update(userData)
		})
		return
	}
	// Обновляем существующую запись
	err = u.update(userData)
	return
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

func (u *User) update(userData UserData) (err error) {
	err = u.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(*u.bucket)
		encoded, err := encodeUser(userData)
		if err != nil {
			return err
		}
		return b.Put([]byte(userData.Email), encoded)
	})
	return err
}
