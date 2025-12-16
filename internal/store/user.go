package store

import (
	"botDashboard/pkg/db"
	"encoding/json"
	"fmt"
	"log"

	"botDashboard/internal/model"

	"go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

type UserRepository struct {
	repo *db.Repository
}

func GetUserRepository() *UserRepository {
	return &UserRepository{
		repo: db.GetRepository(),
	}
}

func (ur *UserRepository) CreateUser(login, email, password string) (model.UserData, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.UserData{}, fmt.Errorf("failed to hash password: %w", err)
	}

	user := model.UserData{
		Login:          login,
		Email:          email,
		HashedPassword: hash,
		IsAdmin:        false,
	}

	// Проверяем существование
	if _, err := ur.FindUserByEmail(email); err == nil {
		return model.UserData{}, fmt.Errorf("user with email %s already exists", email)
	}

	data, err := json.Marshal(user)
	if err != nil {
		return model.UserData{}, fmt.Errorf("failed to marshal user: %w", err)
	}

	err = ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserBucket)
		return b.Put([]byte(email), data)
	})
	if err != nil {
		return model.UserData{}, fmt.Errorf("failed to save user: %w", err)
	}

	log.Printf("Created user: %s", email)
	return user, nil
}

func (ur *UserRepository) FindUserByEmail(email string) (model.UserData, error) {
	var user model.UserData
	err := ur.repo.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserBucket)
		data := b.Get([]byte(email))
		if data == nil {
			return fmt.Errorf("user not found")
		}
		return json.Unmarshal(data, &user)
	})
	if err != nil {
		return model.UserData{}, err
	}
	return user, nil
}

func (ur *UserRepository) UpdateUser(userData model.UserData, prevEmail string) error {
	data, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	return ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserBucket)

		// Удаляем старую запись если email изменился
		if prevEmail != "" && prevEmail != userData.Email {
			if err := b.Delete([]byte(prevEmail)); err != nil {
				log.Printf("failed to delete old user record %s: %v", prevEmail, err)
			}
		}

		return b.Put([]byte(userData.Email), data)
	})
}

func (ur *UserRepository) DeleteUser(email string) error {
	return ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserBucket)
		if err := b.Delete([]byte(email)); err != nil {
			return fmt.Errorf("failed to delete user %s: %w", email, err)
		}
		return nil
	})
}
