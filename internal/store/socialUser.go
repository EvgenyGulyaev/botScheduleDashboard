package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

const OPTIMAL_MESSAGE_SIZE = 10

type SocialUserRepository struct {
	repo *db.Repository
}

func GetSocialUserRepository() *SocialUserRepository {
	return &SocialUserRepository{
		repo: db.GetRepository(),
	}
}

func (sr *SocialUserRepository) FindByID(id int64, network string) (model.SocialUser, error) {
	var user model.SocialUser
	key := socialUserKey(id, network)

	err := sr.repo.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(SocialBucket)
		data := b.Get(key)
		if data == nil {
			return fmt.Errorf("social user not found")
		}
		return json.Unmarshal(data, &user)
	})
	if err != nil {
		return model.SocialUser{}, err
	}
	return user, nil
}

func (sr *SocialUserRepository) CreateSocialUser(id int64, net, name string, mesId int, mesText string) (model.SocialUser, error) {
	u := model.SocialUser{
		Id:       id,
		Net:      net,
		Name:     name,
		Messages: make(map[int]string),
	}
	u.Messages[mesId] = mesText
	// Проверяем существование
	if _, err := sr.FindByID(id, net); err == nil {
		return model.SocialUser{}, fmt.Errorf("social user %s:%d already exists", net, id)
	}

	data, err := json.Marshal(u)
	if err != nil {
		return model.SocialUser{}, fmt.Errorf("failed to marshal social user: %w", err)
	}
	key := socialUserKey(id, net)
	err = sr.repo.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(SocialBucket)
		return b.Put(key, data)
	})
	if err != nil {
		return model.SocialUser{}, fmt.Errorf("failed to save social user: %w", err)
	}
	return u, nil
}

func (sr *SocialUserRepository) ListAll() ([]model.SocialUser, error) {
	result := make([]model.SocialUser, 0)

	err := sr.repo.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(SocialBucket)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var su model.SocialUser
			if err := json.Unmarshal(v, &su); err != nil {
				continue
			}

			result = append(result, su)
		}
		return nil
	})

	return result, err
}

func socialUserKey(id int64, network string) []byte {
	return []byte(fmt.Sprintf("%s:%d", network, id))
}

func (sr *SocialUserRepository) ClearAll() error {
	return sr.repo.ClearBucket(SocialBucket)
}

func (sr *SocialUserRepository) UpdateUserMessages(userData model.SocialUser) error {
	data, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user social: %w", err)
	}

	return sr.repo.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(SocialBucket)
		key := socialUserKey(userData.Id, userData.Net)
		return b.Put(key, data)
	})
}

func (sr *SocialUserRepository) DeleteUserMessage(userData model.SocialUser) error {
	userData.Messages = make(map[int]string)
	return sr.UpdateUserMessages(userData)
}

func (sr *SocialUserRepository) OptimizeUserMessage(userData model.SocialUser) error {
	if len(userData.Messages) >= OPTIMAL_MESSAGE_SIZE {
		return sr.DeleteUserMessage(userData)
	}
	return nil
}
