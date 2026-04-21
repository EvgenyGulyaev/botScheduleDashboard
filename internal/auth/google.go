package auth

import (
	"botDashboard/internal/config"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type GoogleIdentity struct {
	Email         string
	EmailVerified bool
	Name          string
}

type googleTokenInfoResponse struct {
	Audience      string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
}

var tokenInfoEndpoint = "https://oauth2.googleapis.com/tokeninfo"

func GoogleClientID() string {
	if value := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")); value != "" {
		return value
	}
	return strings.TrimSpace(config.LoadConfig().Env["GOOGLE_CLIENT_ID"])
}

func GoogleEnabled() bool {
	return GoogleClientID() != ""
}

func VerifyGoogleIDToken(idToken string) (GoogleIdentity, error) {
	clientID := GoogleClientID()
	if clientID == "" {
		return GoogleIdentity{}, fmt.Errorf("google auth is not configured")
	}
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return GoogleIdentity{}, fmt.Errorf("google id token is required")
	}

	reqURL := tokenInfoEndpoint + "?id_token=" + url.QueryEscape(idToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return GoogleIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GoogleIdentity{}, fmt.Errorf("google token validation failed")
	}

	var payload googleTokenInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return GoogleIdentity{}, err
	}
	if strings.TrimSpace(payload.Audience) != clientID {
		return GoogleIdentity{}, fmt.Errorf("google audience mismatch")
	}
	if strings.TrimSpace(payload.Email) == "" {
		return GoogleIdentity{}, fmt.Errorf("google token email is missing")
	}
	if strings.ToLower(strings.TrimSpace(payload.EmailVerified)) != "true" {
		return GoogleIdentity{}, fmt.Errorf("google email is not verified")
	}

	return GoogleIdentity{
		Email:         strings.TrimSpace(payload.Email),
		EmailVerified: true,
		Name:          strings.TrimSpace(payload.Name),
	}, nil
}
