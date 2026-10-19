package routes_test

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestWeddingMediaSilverliningGuestCookie(t *testing.T) {
	chatHTTPSetup(t)
	repo := store.GetWeddingRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatal(err)
	}
	settings := model.WeddingSettings{DrinkOptions: model.DefaultWeddingDrinkOptions(), AccessCodeEnabled: true, AccessCode: "123456"}
	if _, err := repo.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	resp, data := doJSONRequest(t, "GET", "/wedding/media/status", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, data)
	}
	req, _ := http.NewRequest("POST", chatHTTPURL+"/wedding/access/verify", bytes.NewBufferString(`{"code":"123456"}`))
	req.Header.Set("Origin", chatHTTPURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(resp.Cookies()) != 1 {
		t.Fatalf("verify %d %s cookies %v", resp.StatusCode, data, resp.Cookies())
	}
	cookie := resp.Cookies()[0]
	if !cookie.HttpOnly {
		t.Fatal("not HttpOnly")
	}
	req, _ = http.NewRequest("GET", chatHTTPURL+"/wedding/media/status", nil)
	req.AddCookie(cookie)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Authorized bool `json:"guest_authorized"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.Authorized {
		t.Fatal("cookie not recognized by silverlining adapter")
	}
	req, _ = http.NewRequest("POST", chatHTTPURL+"/wedding/access/verify", bytes.NewBufferString(`{"code":"123456"}`))
	req.Header.Set("Origin", "https://evil.test")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("foreign Origin", resp.StatusCode)
	}
	// Existing nginx may omit X-Forwarded-Proto before the Drive rollout.
	req, _ = http.NewRequest("POST", chatHTTPURL+"/wedding/access/verify", bytes.NewBufferString(`{"code":"123456"}`))
	req.Header.Set("Origin", "https://"+req.URL.Host)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("legacy HTTPS proxy verification", resp.StatusCode)
	}
}
