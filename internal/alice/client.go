package alice

import (
	"botDashboard/internal/config"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Account struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Provider     string    `json:"provider"`
	IsActive     bool      `json:"is_active"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

type Room struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	HouseholdID string `json:"household_id"`
	Name        string `json:"name"`
}

type Device struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	RoomID      string `json:"room_id"`
	HouseholdID string `json:"household_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Online      bool   `json:"online"`
}

type Scenario struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	HouseholdID string `json:"household_id"`
	Name        string `json:"name"`
	IsActive    bool   `json:"is_active"`
}

type Resources struct {
	Rooms     []Room     `json:"rooms"`
	Devices   []Device   `json:"devices"`
	Scenarios []Scenario `json:"scenarios"`
}

type AnnounceRequest struct {
	AccountID      string `json:"account_id"`
	DeviceID       string `json:"device_id"`
	ScenarioID     string `json:"scenario_id"`
	InitiatorEmail string `json:"initiator_email"`
	RecipientEmail string `json:"recipient_email"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Text           string `json:"text"`
}

type AnnounceResponse struct {
	Status     string `json:"status"`
	RequestID  string `json:"request_id"`
	DeliveryID string `json:"delivery_id"`
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient() *Client {
	cfg := config.LoadConfig()
	return &Client{
		baseURL: strings.TrimSpace(readEnv("ALICE_SERVICE_URL", cfg.Env)),
		token:   strings.TrimSpace(readEnv("ALICE_SERVICE_TOKEN", cfg.Env)),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func readEnv(key string, env map[string]string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return strings.TrimSpace(env[key])
}

func (c *Client) Enabled() bool {
	return c.baseURL != ""
}

func (c *Client) ListAccounts() ([]Account, error) {
	var response struct {
		Items []Account `json:"items"`
	}
	if err := c.doJSON(http.MethodGet, "/api/accounts", nil, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (c *Client) GetAccountResources(accountID string) (Resources, error) {
	var response Resources
	if err := c.doJSON(http.MethodGet, "/api/accounts/"+accountID+"/resources", nil, &response); err != nil {
		return Resources{}, err
	}
	return response, nil
}

func (c *Client) AnnounceScenario(payload AnnounceRequest) (AnnounceResponse, error) {
	var response AnnounceResponse
	if err := c.doJSON(http.MethodPost, "/api/announce/scenario", payload, &response); err != nil {
		return AnnounceResponse{}, err
	}
	return response, nil
}

func (c *Client) doJSON(method, path string, body any, target any) error {
	if !c.Enabled() {
		return errors.New("alice service is not configured")
	}

	var requestBody []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		requestBody = raw
	}

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message != "" {
			return errors.New(apiErr.Message)
		}
		return fmt.Errorf("alice service returned status %d", resp.StatusCode)
	}

	if target == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
