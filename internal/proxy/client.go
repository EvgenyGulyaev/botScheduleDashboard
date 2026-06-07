package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Config struct {
	BaseURL string
	Token   string
}

type Response struct {
	Status int
	Data   any
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8095"
	}
	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(cfg.Token),
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func NewClientFromEnv() *Client {
	return NewClient(Config{
		BaseURL: os.Getenv("VPN_GATEWAY_URL"),
		Token:   os.Getenv("VPN_GATEWAY_API_TOKEN"),
	})
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

func (c *Client) Do(ctx context.Context, method, path string, body []byte) (Response, error) {
	if !c.Enabled() {
		return Response{}, errors.New("proxy service is not configured")
	}

	requestURL := c.baseURL + "/api/v1" + ensurePath(path)
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode == http.StatusNoContent {
		return Response{Status: resp.StatusCode, Data: map[string]bool{"ok": true}}, nil
	}

	var data any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return Response{}, fmt.Errorf("proxy service returned invalid json: %w", err)
		}
	} else {
		data = map[string]bool{"ok": true}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Response{}, fmt.Errorf("proxy service returned status %d: %s", resp.StatusCode, extractMessage(data))
	}

	return Response{Status: resp.StatusCode, Data: data}, nil
}

func ensurePath(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func extractMessage(data any) string {
	if payload, ok := data.(map[string]any); ok {
		if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
	}
	return "unknown error"
}
