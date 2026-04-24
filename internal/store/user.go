package store

import (
	"botDashboard/pkg/db"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

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
		Login:                login,
		Email:                email,
		HashedPassword:       hash,
		IsAdmin:              false,
		DefaultApp:           model.DefaultAppChat,
		NotificationSettings: model.DefaultUserNotificationSettings(),
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
	return normalizeUserData(user), nil
}

func (ur *UserRepository) ListAll() ([]model.UserData, error) {
	result := make([]model.UserData, 0)

	err := ur.repo.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserBucket)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var user model.UserData
			if err := json.Unmarshal(v, &user); err != nil {
				continue
			}
			result = append(result, normalizeUserData(user))
		}
		return nil
	})
	sort.Slice(result, func(i, j int) bool {
		if result[i].Login == result[j].Login {
			return result[i].Email < result[j].Email
		}
		return result[i].Login < result[j].Login
	})
	return result, err
}

func (ur *UserRepository) UpdateUser(userData model.UserData, prevEmail string) error {
	userData = normalizeUserData(userData)
	data, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	return ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserBucket)
		pushBucket := tx.Bucket(UserPushSubscriptionsBucket)

		if prevEmail != "" && prevEmail != userData.Email && pushBucket != nil {
			if err := migratePushSubscriptions(pushBucket, prevEmail, userData.Email); err != nil {
				return err
			}
		}

		// Удаляем старую запись если email изменился
		if prevEmail != "" && prevEmail != userData.Email {
			if err := b.Delete([]byte(prevEmail)); err != nil {
				log.Printf("failed to delete old user record %s: %v", prevEmail, err)
			}
		}

		return b.Put([]byte(userData.Email), data)
	})
}

func (ur *UserRepository) HashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func (ur *UserRepository) SavePushSubscription(email string, subscription model.PushSubscription) error {
	email = strings.TrimSpace(email)
	subscription.Endpoint = strings.TrimSpace(subscription.Endpoint)
	subscription.Keys.P256DH = strings.TrimSpace(subscription.Keys.P256DH)
	subscription.Keys.Auth = strings.TrimSpace(subscription.Keys.Auth)
	subscription.UserAgent = strings.TrimSpace(subscription.UserAgent)
	if email == "" || subscription.Endpoint == "" || subscription.Keys.P256DH == "" || subscription.Keys.Auth == "" {
		return fmt.Errorf("push subscription is incomplete")
	}

	now := time.Now().UTC()
	if subscription.CreatedAt.IsZero() {
		subscription.CreatedAt = now
	}
	subscription.UpdatedAt = now

	data, err := json.Marshal(subscription)
	if err != nil {
		return fmt.Errorf("failed to marshal push subscription: %w", err)
	}

	return ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserPushSubscriptionsBucket)
		if b == nil {
			return fmt.Errorf("push subscriptions bucket not found")
		}
		return b.Put([]byte(pushSubscriptionKey(email, subscription.Endpoint)), data)
	})
}

func (ur *UserRepository) ListPushSubscriptions(email string) ([]model.PushSubscription, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, nil
	}

	subscriptions := make([]model.PushSubscription, 0)
	err := ur.repo.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserPushSubscriptionsBucket)
		if b == nil {
			return fmt.Errorf("push subscriptions bucket not found")
		}

		prefix := []byte(pushSubscriptionPrefix(email))
		cursor := b.Cursor()
		for key, value := cursor.Seek(prefix); key != nil && strings.HasPrefix(string(key), string(prefix)); key, value = cursor.Next() {
			var subscription model.PushSubscription
			if err := json.Unmarshal(value, &subscription); err != nil {
				return err
			}
			subscriptions = append(subscriptions, subscription)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(subscriptions, func(i, j int) bool {
		return subscriptions[i].UpdatedAt.After(subscriptions[j].UpdatedAt)
	})
	return subscriptions, nil
}

func (ur *UserRepository) DeletePushSubscription(email, endpoint string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	endpoint = strings.TrimSpace(endpoint)

	return ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(UserPushSubscriptionsBucket)
		if b == nil {
			return fmt.Errorf("push subscriptions bucket not found")
		}
		if endpoint == "" {
			prefix := []byte(pushSubscriptionPrefix(email))
			cursor := b.Cursor()
			for key, _ := cursor.Seek(prefix); key != nil && strings.HasPrefix(string(key), string(prefix)); key, _ = cursor.Next() {
				if err := b.Delete(key); err != nil {
					return err
				}
			}
			return nil
		}
		return b.Delete([]byte(pushSubscriptionKey(email, endpoint)))
	})
}

func normalizeUserData(user model.UserData) model.UserData {
	if !user.NotificationSettings.Configured {
		user.NotificationSettings = model.DefaultUserNotificationSettings()
	}
	user.DefaultApp = model.NormalizeDefaultApp(strings.TrimSpace(user.DefaultApp))
	user.AliceSettings.AccountID = strings.TrimSpace(user.AliceSettings.AccountID)
	user.AliceSettings.HouseholdID = strings.TrimSpace(user.AliceSettings.HouseholdID)
	user.AliceSettings.RoomID = strings.TrimSpace(user.AliceSettings.RoomID)
	user.AliceSettings.DeviceID = strings.TrimSpace(user.AliceSettings.DeviceID)
	user.AliceSettings.ScenarioID = strings.TrimSpace(user.AliceSettings.ScenarioID)
	user.AliceSettings.Voice = strings.TrimSpace(user.AliceSettings.Voice)
	user.AliceSettings.Configured = user.AliceSettings.AccountID != "" || user.AliceSettings.HouseholdID != "" || user.AliceSettings.RoomID != "" || user.AliceSettings.DeviceID != "" || user.AliceSettings.ScenarioID != "" || user.AliceSettings.Voice != ""
	return user
}

func pushSubscriptionPrefix(email string) string {
	return email + "|"
}

func pushSubscriptionKey(email, endpoint string) string {
	return pushSubscriptionPrefix(email) + endpoint
}

func migratePushSubscriptions(bucket *bbolt.Bucket, fromEmail, toEmail string) error {
	fromEmail = strings.TrimSpace(fromEmail)
	toEmail = strings.TrimSpace(toEmail)
	if fromEmail == "" || toEmail == "" || fromEmail == toEmail {
		return nil
	}

	prefix := []byte(pushSubscriptionPrefix(fromEmail))
	cursor := bucket.Cursor()
	keysToDelete := make([][]byte, 0)
	subscriptions := make([]model.PushSubscription, 0)

	for key, value := cursor.Seek(prefix); key != nil && strings.HasPrefix(string(key), string(prefix)); key, value = cursor.Next() {
		var subscription model.PushSubscription
		if err := json.Unmarshal(value, &subscription); err != nil {
			return err
		}
		subscriptions = append(subscriptions, subscription)
		keysToDelete = append(keysToDelete, append([]byte(nil), key...))
	}

	for _, key := range keysToDelete {
		if err := bucket.Delete(key); err != nil {
			return err
		}
	}

	for _, subscription := range subscriptions {
		data, err := json.Marshal(subscription)
		if err != nil {
			return err
		}
		if err := bucket.Put([]byte(pushSubscriptionKey(toEmail, subscription.Endpoint)), data); err != nil {
			return err
		}
	}

	return nil
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

func (ur *UserRepository) ClearAll() error {
	return ur.repo.ClearBucket(UserBucket)
}

func (ur *UserRepository) ClearPasswordResetTokens() error {
	return ur.repo.ClearBucket(PasswordResetTokensBucket)
}

func (ur *UserRepository) CreatePasswordResetToken(email string, ttl time.Duration) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", err
	}
	rawToken := hex.EncodeToString(rawBytes)
	tokenHash := sha256.Sum256([]byte(rawToken))
	key := hex.EncodeToString(tokenHash[:])
	now := time.Now().UTC()
	record := model.PasswordResetToken{
		Email:     email,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}

	if err := ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(PasswordResetTokensBucket)
		if b == nil {
			return fmt.Errorf("password reset bucket not found")
		}
		return b.Put([]byte(key), data)
	}); err != nil {
		return "", err
	}

	return rawToken, nil
}

func (ur *UserRepository) ConsumePasswordResetToken(rawToken string) (model.PasswordResetToken, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return model.PasswordResetToken{}, fmt.Errorf("token is required")
	}

	tokenHash := sha256.Sum256([]byte(rawToken))
	key := hex.EncodeToString(tokenHash[:])
	now := time.Now().UTC()
	var record model.PasswordResetToken

	err := ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(PasswordResetTokensBucket)
		if b == nil {
			return fmt.Errorf("password reset bucket not found")
		}
		data := b.Get([]byte(key))
		if data == nil {
			return fmt.Errorf("reset token not found")
		}
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		if record.UsedAt != nil {
			return fmt.Errorf("reset token already used")
		}
		if !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(now) {
			return fmt.Errorf("reset token expired")
		}
		record.UsedAt = &now
		updated, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), updated)
	})
	if err != nil {
		return model.PasswordResetToken{}, err
	}

	return record, nil
}

func (ur *UserRepository) DeleteExpiredPasswordResetTokens() error {
	now := time.Now().UTC()
	return ur.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(PasswordResetTokensBucket)
		if b == nil {
			return fmt.Errorf("password reset bucket not found")
		}
		cursor := b.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			var record model.PasswordResetToken
			if err := json.Unmarshal(value, &record); err != nil {
				return err
			}
			if record.UsedAt != nil || (!record.ExpiresAt.IsZero() && !record.ExpiresAt.After(now)) {
				if err := b.Delete(key); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
