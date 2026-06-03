package drawing

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	BaseURL      string
	ServiceToken string
}

func LoadConfigFromEnv() (Config, error) {
	baseURL := strings.TrimSpace(os.Getenv("DRAWING_SERVICE_BASE_URL"))
	if baseURL == "" {
		return Config{}, errors.New("DRAWING_SERVICE_BASE_URL is required")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	token := strings.TrimSpace(os.Getenv("DRAWING_SERVICE_TOKEN"))
	if token == "" {
		return Config{}, errors.New("DRAWING_SERVICE_TOKEN is required")
	}
	return Config{BaseURL: baseURL, ServiceToken: token}, nil
}
