// wedding-drive-auth runs locally on the Drive owner's computer, never as a public endpoint.
package main

import (
	"botDashboard/internal/weddingmedia"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type callbackResult struct {
	code string
	err  error
}

func nonce() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func authorizationURL(client, redirect, state, verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	q := url.Values{"client_id": {client}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"https://www.googleapis.com/auth/drive.file"}, "access_type": {"offline"}, "prompt": {"consent"}, "state": {state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()
}
func callbackHandler(state string, results chan<- callbackResult) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if r.Method != "GET" || r.URL.Path != "/callback" || r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid callback state", 400)
			return
		}
		result := callbackResult{code: r.URL.Query().Get("code")}
		if result.code == "" {
			result.err = errors.New("Google authorization was cancelled")
		}
		select {
		case results <- result:
			fmt.Fprint(w, "Google Drive authorization received. You can close this window.")
		default:
			http.Error(w, "Authorization already received", 409)
		}
	})
}
func readJSON(r io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("response too large")
	}
	return json.Unmarshal(data, target)
}
func run() error {
	clientPath := flag.String("client", "", "downloaded Desktop OAuth client JSON (kept local)")
	output := flag.String("out", "", "absolute protected configuration path, outside repository")
	origin := flag.String("origin", "", "canonical wedding site origin, e.g. https://wedding.example")
	folder := flag.String("folder", "Wedding photos and videos", "new private Drive folder name")
	flag.Parse()
	if *clientPath == "" || *output == "" || !strings.HasPrefix(*output, "/") {
		return errors.New("provide -client, absolute -out and -origin; never paste client credentials into chat")
	}
	u, err := url.Parse(*origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("-origin must be canonical https origin without trailing slash")
	}
	if _, err = os.Stat(*output); err == nil {
		return errors.New("output already exists; preserve it and choose a new protected path to reconnect")
	}
	f, err := os.Open(*clientPath)
	if err != nil {
		return errors.New("cannot read OAuth client file")
	}
	var doc struct {
		Installed struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
	}
	err = readJSON(f, &doc)
	f.Close()
	if err != nil || doc.Installed.ClientID == "" || doc.Installed.ClientSecret == "" {
		return errors.New("expected a downloaded Desktop app OAuth client JSON")
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	redirect := "http://" + ln.Addr().String() + "/callback"
	state, verifier := nonce(), nonce()
	results := make(chan callbackResult, 1)
	server := &http.Server{Handler: callbackHandler(state, results), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	defer server.Close()
	go server.Serve(ln)
	fmt.Println("Open this URL on this computer and authorize your personal Google account:")
	fmt.Println(authorizationURL(doc.Installed.ClientID, redirect, state, verifier))
	var result callbackResult
	select {
	case result = <-results:
	case <-time.After(10 * time.Minute):
		return errors.New("authorization timed out; run again")
	}
	if result.err != nil {
		return result.err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := url.Values{"client_id": {doc.Installed.ClientID}, "client_secret": {doc.Installed.ClientSecret}, "code": {result.code}, "redirect_uri": {redirect}, "code_verifier": {verifier}, "grant_type": {"authorization_code"}}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(req)
	if err != nil {
		return errors.New("Google token exchange failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("Google declined token exchange; check OAuth client and consent screen")
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err = readJSON(response.Body, &token); err != nil || token.AccessToken == "" || token.RefreshToken == "" {
		return errors.New("offline refresh token missing; revoke app consent and authorize again")
	}
	body, _ := json.Marshal(map[string]string{"name": *folder, "mimeType": "application/vnd.google-apps.folder"})
	req, _ = http.NewRequestWithContext(ctx, "POST", "https://www.googleapis.com/drive/v3/files?fields=id", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	response, err = client.Do(req)
	if err != nil {
		return errors.New("Drive folder creation failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return errors.New("cannot create Drive folder; check API activation and available storage")
	}
	var created struct {
		ID string `json:"id"`
	}
	if err = readJSON(response.Body, &created); err != nil || created.ID == "" {
		return errors.New("invalid Drive folder response")
	}
	cfg := weddingmedia.Config{ClientID: doc.Installed.ClientID, ClientSecret: doc.Installed.ClientSecret, RefreshToken: token.RefreshToken, FolderID: created.ID, CookieKey: nonce() + nonce(), Origin: *origin}
	if err = weddingmedia.SaveConfig(*output, cfg); err != nil {
		return fmt.Errorf("cannot write protected configuration: %w", err)
	}
	fmt.Println("Saved protected Drive configuration:", *output)
	fmt.Println("Private folder:", "https://drive.google.com/drive/folders/"+created.ID)
	fmt.Println("Securely copy this file to the backend server and set WEDDING_DRIVE_CONFIG. Never commit it or paste its contents into chat.")
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
