package routes

import (
	"botDashboard/internal/config"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"crypto/rand"
	"encoding/hex"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
)

func appBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("APP_BASE_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return strings.TrimRight(strings.TrimSpace(config.LoadConfig().Env["APP_BASE_URL"]), "/")
}

func passwordResetTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("PASSWORD_RESET_TTL_MINUTES"))
	if raw == "" {
		raw = strings.TrimSpace(config.LoadConfig().Env["PASSWORD_RESET_TTL_MINUTES"])
	}
	if raw == "" {
		return 30 * time.Minute
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 30 * time.Minute
	}
	return time.Duration(value) * time.Minute
}

func normalizeGoogleLogin(name, email string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if address, err := mail.ParseAddress(email); err == nil && address.Address != "" {
		email = address.Address
	}
	email = strings.TrimSpace(email)
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return "google-user"
}

func upsertGoogleUser(email, name string) (model.UserData, error) {
	repo := store.GetUserRepository()
	user, err := repo.FindUserByEmail(email)
	if err == nil {
		if strings.TrimSpace(user.Login) == "" {
			user.Login = normalizeGoogleLogin(name, email)
			if updateErr := repo.UpdateUser(user, user.Email); updateErr != nil {
				return model.UserData{}, updateErr
			}
		}
		return user, nil
	}

	return repo.CreateUser(normalizeGoogleLogin(name, email), email, randomTemporaryPassword())
}

func randomTemporaryPassword() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "temporary-google-password"
	}
	return hex.EncodeToString(buffer)
}
