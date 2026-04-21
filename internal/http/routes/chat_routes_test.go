package routes_test

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	httpserver "botDashboard/internal/http"
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"botDashboard/pkg/db"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	nethttp "net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-www/silverlining"
	bolt "go.etcd.io/bbolt"
)

type chatRoutesPublisher struct {
	subjects []string
	payloads []any
}

func (p *chatRoutesPublisher) Publish(subject string, payload any) error {
	p.subjects = append(p.subjects, subject)
	p.payloads = append(p.payloads, payload)
	return nil
}

var (
	chatHTTPOnce sync.Once
	chatHTTPURL  string
)

func chatHTTPSetup(t *testing.T) {
	t.Helper()

	chatHTTPOnce.Do(func() {
		dir, err := os.MkdirTemp("", "chat-http-test-*")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MIDDLEWARE_OFF=false\n"), 0600); err != nil {
			panic(err)
		}
		if err := os.Chdir(dir); err != nil {
			panic(err)
		}

		if err := os.Setenv("DB_NAME_FILE", filepath.Join(dir, "chat-http.db")); err != nil {
			panic(err)
		}
		if err := os.Setenv("JWT_KEY", "chat-http-test-key"); err != nil {
			panic(err)
		}

		store.InitStore()

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		chatHTTPURL = "http://" + ln.Addr().String()

		srv := &silverlining.Server{Handler: httpserver.HandleRequest}
		go func() {
			_ = srv.Serve(ln)
		}()
	})

	_ = store.GetUserRepository().ClearAll()
	_ = store.GetUserRepository().ClearPasswordResetTokens()
	_ = store.GetChatRepository().ClearAll()
}

func createTestUser(t *testing.T, login, email string) model.UserData {
	t.Helper()

	u, err := store.GetUserRepository().CreateUser(login, email, "password")
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u
}

func authToken(t *testing.T, email, login string) string {
	t.Helper()

	token, err := middleware.GetJwt().CreateToken(email, login)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

func doJSONRequest(t *testing.T, method, path, token string, body any) (*nethttp.Response, []byte) {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}

	req, err := nethttp.NewRequest(method, chatHTTPURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, data
}

func doMultipartAudioRequestWithHeaders(t *testing.T, path, token, chatToken string, duration string, payload []byte) (*nethttp.Response, []byte) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("duration_seconds", duration); err != nil {
		t.Fatalf("write duration field: %v", err)
	}
	part, err := writer.CreateFormFile("audio", "voice.webm")
	if err != nil {
		t.Fatalf("create audio form file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write audio payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := nethttp.NewRequest(nethttp.MethodPost, chatHTTPURL+path, &body)
	if err != nil {
		t.Fatalf("new multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if chatToken != "" {
		req.Header.Set(middleware.ChatTokenHeader, chatToken)
	}

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do multipart request: %v", err)
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read multipart response: %v", err)
	}
	return resp, data
}

func doMultipartAudioRequest(t *testing.T, path, token string, duration string, payload []byte) (*nethttp.Response, []byte) {
	t.Helper()
	return doMultipartAudioRequestWithHeaders(t, path, token, "", duration, payload)
}

func doMultipartImageRequest(t *testing.T, path, token string, payload []byte, filename, contentType string) (*nethttp.Response, []byte) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create image form file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write image payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := nethttp.NewRequest(nethttp.MethodPost, chatHTTPURL+path, &body)
	if err != nil {
		t.Fatalf("new multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do multipart request: %v", err)
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read multipart response: %v", err)
	}
	return resp, data
}

type chatUserResponse struct {
	Login   string `json:"login"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type chatReceiptResponse struct {
	Login string `json:"login"`
	Email string `json:"email"`
}

type chatMessageResponse struct {
	ID               string                    `json:"id"`
	Type             string                    `json:"type"`
	Text             string                    `json:"text"`
	SenderLogin      string                    `json:"sender_login"`
	ReplyToMessageID string                    `json:"reply_to_message_id"`
	EditedAt         *time.Time                `json:"edited_at"`
	DeliveredTo      []chatReceiptResponse     `json:"delivered_to"`
	ReadBy           []chatReceiptResponse     `json:"read_by"`
	Audio            *chatAudioResponse        `json:"audio"`
	Image            *chatImageResponse        `json:"image"`
	Call             *chatCallMessageResponse  `json:"call"`
	ReplyPreview     *chatReplyPreviewResponse `json:"reply_preview"`
	Reactions        []chatReactionResponse    `json:"reactions"`
}

type chatReplyPreviewResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Text        string `json:"text"`
	SenderLogin string `json:"sender_login"`
}

type chatReactionResponse struct {
	Emoji     string `json:"emoji"`
	UserEmail string `json:"user_email"`
	UserLogin string `json:"user_login"`
}

type chatAudioResponse struct {
	ID              string `json:"id"`
	MimeType        string `json:"mime_type"`
	SizeBytes       int64  `json:"size_bytes"`
	DurationSeconds int    `json:"duration_seconds"`
	Consumed        bool   `json:"consumed"`
	ConsumedByEmail string `json:"consumed_by_email"`
	ConsumedByLogin string `json:"consumed_by_login"`
	Expired         bool   `json:"expired"`
}

type chatImageResponse struct {
	ID              string `json:"id"`
	MimeType        string `json:"mime_type"`
	SizeBytes       int64  `json:"size_bytes"`
	Consumed        bool   `json:"consumed"`
	ConsumedByEmail string `json:"consumed_by_email"`
	ConsumedByLogin string `json:"consumed_by_login"`
	Expired         bool   `json:"expired"`
}

type chatMemberResponse struct {
	Login             string `json:"login"`
	Email             string `json:"email"`
	LastReadMessageID string `json:"last_read_message_id"`
}

type chatConversationResponse struct {
	ID              string                    `json:"id"`
	CreatedByEmail  string                    `json:"created_by_email"`
	CreatedByLogin  string                    `json:"created_by_login"`
	Title           string                    `json:"title"`
	UnreadCount     int                       `json:"unread_count"`
	Messages        []chatMessageResponse     `json:"messages"`
	Members         []chatMemberResponse      `json:"members"`
	PinnedMessageID string                    `json:"pinned_message_id"`
	PinnedMessage   *chatReplyPreviewResponse `json:"pinned_message"`
}

type chatSearchResultResponse struct {
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	MessageID         string `json:"message_id"`
	SenderLogin       string `json:"sender_login"`
	Text              string `json:"text"`
}

type chatCallParticipantResponse struct {
	Email string `json:"email"`
	Login string `json:"login"`
	Muted bool   `json:"muted"`
}

type chatCallResponse struct {
	ID              string                        `json:"id"`
	ConversationID  string                        `json:"conversation_id"`
	MessageID       string                        `json:"message_id"`
	StartedByEmail  string                        `json:"started_by_email"`
	StartedByLogin  string                        `json:"started_by_login"`
	MaxParticipants int                           `json:"max_participants"`
	Participants    []chatCallParticipantResponse `json:"participants"`
	EndedAt         *time.Time                    `json:"ended_at"`
}

type chatCallMessageResponse struct {
	CallID           string     `json:"call_id"`
	Joinable         bool       `json:"joinable"`
	ParticipantCount int        `json:"participant_count"`
	EndedAt          *time.Time `json:"ended_at"`
}

type chatCallConfigResponse struct {
	IceServers []struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username"`
		Credential string   `json:"credential"`
	} `json:"ice_servers"`
}

func TestGetChatUsers(t *testing.T) {
	chatHTTPSetup(t)

	alice := createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	alice.IsAdmin = true
	if err := store.GetUserRepository().UpdateUser(alice, alice.Email); err != nil {
		t.Fatalf("update alice admin flag: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/chat/users", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var users []chatUserResponse
	if err := json.Unmarshal(data, &users); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	logins := []string{users[0].Login, users[1].Login}
	sort.Strings(logins)
	if strings.Join(logins, ",") != "alice,bob" {
		t.Fatalf("unexpected users: %#v", users)
	}
	var foundAdmin bool
	for _, user := range users {
		if user.Email == "alice@example.com" {
			foundAdmin = true
			if !user.IsAdmin {
				t.Fatalf("expected alice to be admin, got %#v", user)
			}
		}
	}
	if !foundAdmin {
		t.Fatalf("alice not found in response: %#v", users)
	}
}

func TestAuthenticatedChatRequestRefreshesSessionToken(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/chat/users", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	refreshedToken := resp.Header.Get("X-Auth-Token")
	if refreshedToken == "" {
		t.Fatal("expected refreshed auth token header")
	}
	if expose := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(expose, "X-Auth-Token") {
		t.Fatalf("expected X-Auth-Token to be exposed for browsers, got %q", expose)
	}

	claims, err := middleware.GetJwt().ValidateToken(refreshedToken)
	if err != nil {
		t.Fatalf("validate refreshed token: %v", err)
	}
	if claims.Email != "alice@example.com" || claims.Login != "alice" {
		t.Fatalf("unexpected refreshed token claims: %#v", claims)
	}
}

func TestPostChatGroupAndGetChatConversations(t *testing.T) {
	chatHTTPSetup(t)

	alice := createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "carol", "carol@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com", "carol@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatConversationResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Title != "Team chat" {
		t.Fatalf("unexpected title: %#v", created)
	}
	if created.CreatedByEmail != alice.Email || created.CreatedByLogin != alice.Login {
		t.Fatalf("expected creator to be current user, got %#v", created)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, "/chat/conversations", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var conversations []chatConversationResponse
	if err := json.Unmarshal(data, &conversations); err != nil {
		t.Fatalf("decode conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected one conversation, got %#v", conversations)
	}
	if conversations[0].Title != "Team chat" {
		t.Fatalf("unexpected conversation payload: %#v", conversations[0])
	}
}

func TestGetChatMessages(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/messages", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID || messages[0].Text != "hello" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestGetChatMessagesIncludesReplyAndEditFields(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	source, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "source")
	if err != nil {
		t.Fatalf("add source message: %v", err)
	}

	reply, err := store.GetChatRepository().AddMessage(conv.ID, "alice@example.com", "alice", "reply")
	if err != nil {
		t.Fatalf("add reply message: %v", err)
	}

	editedAt := time.Now().UTC().Round(time.Second)
	err = db.GetRepository().Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(store.ChatMessagesBucket)
		if bucket == nil {
			t.Fatal("chat messages bucket not found")
		}

		reply.ReplyToMessageID = source.ID
		reply.UpdatedAt = editedAt
		reply.EditedAt = &editedAt

		raw, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(reply.ConversationID+"|"+reply.ID), raw)
	})
	if err != nil {
		t.Fatalf("seed reply metadata: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/messages", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %#v", messages)
	}

	var found chatMessageResponse
	for _, message := range messages {
		if message.ID == reply.ID {
			found = message
			break
		}
	}
	if found.ID == "" {
		t.Fatalf("reply message not found in payload: %#v", messages)
	}
	if found.ReplyToMessageID != source.ID {
		t.Fatalf("expected reply_to_message_id %q, got %#v", source.ID, found)
	}
	if found.EditedAt == nil || !found.EditedAt.Equal(editedAt) {
		t.Fatalf("expected edited_at %v, got %#v", editedAt, found)
	}
	if found.ReplyPreview == nil || found.ReplyPreview.ID != source.ID || found.ReplyPreview.Text != "source" {
		t.Fatalf("expected reply preview for source message, got %#v", found)
	}
}

func TestPostChatAudioAndConsumeOnce(t *testing.T) {
	chatHTTPSetup(t)

	audioDir := t.TempDir()
	store.ConfigureChatAudio(audioDir, "60", "10")
	t.Cleanup(func() {
		store.ConfigureChatAudio("", "", "")
		producer.ResetPublisherForTest()
	})
	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	audioPayload := []byte("webm-audio-data")
	resp, data := doMultipartAudioRequest(
		t,
		fmt.Sprintf("/chat/conversations/%s/audio", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		"7",
		audioPayload,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatMessageResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode created audio: %v", err)
	}
	if created.Type != "audio" || created.Audio == nil || created.Audio.DurationSeconds != 7 {
		t.Fatalf("unexpected audio response: %#v", created)
	}
	if len(pub.subjects) != 1 || pub.subjects[0] != event.ChatEventMessagePersisted {
		t.Fatalf("expected persisted event publish, got %#v", pub.subjects)
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/audio", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on audio consume, got %d: %s", resp.StatusCode, string(data))
	}
	if string(data) != string(audioPayload) {
		t.Fatalf("unexpected audio payload: %q", string(data))
	}
	entries, err := os.ReadDir(audioDir)
	if err != nil {
		t.Fatalf("read audio dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected audio file to be removed after consume, got %d files", len(entries))
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 when reloading messages, got %d: %s", resp.StatusCode, string(data))
	}

	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages after audio consume: %v", err)
	}
	if len(messages) != 1 || messages[0].Audio == nil {
		t.Fatalf("unexpected messages after audio consume: %#v", messages)
	}
	if !messages[0].Audio.Consumed || messages[0].Audio.ConsumedByEmail != "bob@example.com" || messages[0].Audio.ConsumedByLogin != "bob" {
		t.Fatalf("expected audio consume metadata to be visible, got %#v", messages[0].Audio)
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/audio", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusGone {
		t.Fatalf("expected 410 on second consume, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostChatAudioAcceptsChatTokenHeader(t *testing.T) {
	chatHTTPSetup(t)

	audioDir := t.TempDir()
	store.ConfigureChatAudio(audioDir, "60", "10")
	t.Cleanup(func() {
		store.ConfigureChatAudio("", "", "")
		producer.ResetPublisherForTest()
	})
	producer.SetPublisherForTest(&chatRoutesPublisher{})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	resp, data := doMultipartAudioRequestWithHeaders(
		t,
		fmt.Sprintf("/chat/conversations/%s/audio", conv.ID),
		"",
		authToken(t, "alice@example.com", "alice"),
		"4",
		[]byte("webm-audio-data"),
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 with %s header, got %d: %s", middleware.ChatTokenHeader, resp.StatusCode, string(data))
	}
}

func TestPostChatAudioAcceptsQueryToken(t *testing.T) {
	chatHTTPSetup(t)

	audioDir := t.TempDir()
	store.ConfigureChatAudio(audioDir, "60", "10")
	t.Cleanup(func() {
		store.ConfigureChatAudio("", "", "")
		producer.ResetPublisherForTest()
	})
	producer.SetPublisherForTest(&chatRoutesPublisher{})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	token := authToken(t, "alice@example.com", "alice")
	resp, data := doMultipartAudioRequestWithHeaders(
		t,
		fmt.Sprintf("/chat/conversations/%s/audio?token=%s", conv.ID, token),
		"",
		"",
		"4",
		[]byte("webm-audio-data"),
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 with query token, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostChatAudioAcceptsPathToken(t *testing.T) {
	chatHTTPSetup(t)

	audioDir := t.TempDir()
	store.ConfigureChatAudio(audioDir, "60", "10")
	t.Cleanup(func() {
		store.ConfigureChatAudio("", "", "")
		producer.ResetPublisherForTest()
	})
	producer.SetPublisherForTest(&chatRoutesPublisher{})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	token := authToken(t, "alice@example.com", "alice")
	resp, data := doMultipartAudioRequestWithHeaders(
		t,
		fmt.Sprintf("/chat/conversations/%s/audio/%s", conv.ID, token),
		"",
		"",
		"4",
		[]byte("webm-audio-data"),
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 with path token, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestGetChatAudioExpiresAfterDeadline(t *testing.T) {
	chatHTTPSetup(t)

	audioDir := t.TempDir()
	store.ConfigureChatAudio(audioDir, "60", "10")
	previousTTL := store.CHAT_AUDIO_TTL
	store.CHAT_AUDIO_TTL = 10 * time.Millisecond
	t.Cleanup(func() {
		store.ConfigureChatAudio("", "", "")
		store.CHAT_AUDIO_TTL = previousTTL
		producer.ResetPublisherForTest()
	})
	producer.SetPublisherForTest(&chatRoutesPublisher{})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	audioPayload := []byte("webm-audio-data")
	resp, data := doMultipartAudioRequest(
		t,
		fmt.Sprintf("/chat/conversations/%s/audio", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		"7",
		audioPayload,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatMessageResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode created audio: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/audio", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusGone {
		t.Fatalf("expected 410 on expired audio, got %d: %s", resp.StatusCode, string(data))
	}

	entries, err := os.ReadDir(audioDir)
	if err != nil {
		t.Fatalf("read audio dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected expired audio file to be removed, got %d files", len(entries))
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 when reloading messages, got %d: %s", resp.StatusCode, string(data))
	}

	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages after expiration: %v", err)
	}
	if len(messages) != 1 || messages[0].Audio == nil || !messages[0].Audio.Expired {
		t.Fatalf("expected expired audio metadata, got %#v", messages)
	}
}

func TestPostChatAudioPreservesUploadedBytes(t *testing.T) {
	chatHTTPSetup(t)

	audioDir := t.TempDir()
	store.ConfigureChatAudio(audioDir, "60", "10")
	t.Cleanup(func() {
		store.ConfigureChatAudio("", "", "")
		producer.ResetPublisherForTest()
	})
	producer.SetPublisherForTest(&chatRoutesPublisher{})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	audioPayload := bytes.Repeat([]byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x7f, 0x11, 0xa4}, 4096)
	token := authToken(t, "alice@example.com", "alice")
	resp, data := doMultipartAudioRequest(
		t,
		fmt.Sprintf("/chat/conversations/%s/audio/%s", conv.ID, token),
		"",
		"7",
		audioPayload,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatMessageResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode created audio: %v", err)
	}
	if created.Audio == nil {
		t.Fatalf("expected audio metadata in response, got %#v", created)
	}
	if created.Audio.SizeBytes != int64(len(audioPayload)) {
		t.Fatalf("expected stored audio size %d, got %d", len(audioPayload), created.Audio.SizeBytes)
	}

	resp, consumed := doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/audio", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 when consuming audio, got %d: %s", resp.StatusCode, string(consumed))
	}
	if !bytes.Equal(consumed, audioPayload) {
		t.Fatalf("expected consumed audio bytes to match uploaded payload: got %d want %d", len(consumed), len(audioPayload))
	}
}

func TestPostChatImageAndConsumeOnce(t *testing.T) {
	chatHTTPSetup(t)

	imageDir := t.TempDir()
	store.ConfigureChatImage(imageDir, "10")
	t.Cleanup(func() {
		store.ConfigureChatImage("", "")
		producer.ResetPublisherForTest()
	})
	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	imagePayload := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	resp, data := doMultipartImageRequest(
		t,
		fmt.Sprintf("/chat/conversations/%s/image", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		imagePayload,
		"photo.png",
		"image/png",
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatMessageResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode created image: %v", err)
	}
	if created.Type != "image" || created.Image == nil || created.Image.MimeType != "image/png" {
		t.Fatalf("unexpected image response: %#v", created)
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/image", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on image consume, got %d: %s", resp.StatusCode, string(data))
	}
	if string(data) != string(imagePayload) {
		t.Fatalf("unexpected image payload: %v", data)
	}
	entries, err := os.ReadDir(imageDir)
	if err != nil {
		t.Fatalf("read image dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected image file to be removed after consume, got %d files", len(entries))
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 when reloading messages, got %d: %s", resp.StatusCode, string(data))
	}
	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages after image consume: %v", err)
	}
	if len(messages) != 1 || messages[0].Image == nil || !messages[0].Image.Consumed || messages[0].Image.ConsumedByEmail != "bob@example.com" {
		t.Fatalf("expected image consume metadata to be visible, got %#v", messages)
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/image", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusGone {
		t.Fatalf("expected 410 on second image consume, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestGetChatImageExpiresAfterDeadline(t *testing.T) {
	chatHTTPSetup(t)

	imageDir := t.TempDir()
	store.ConfigureChatImage(imageDir, "10")
	previousTTL := store.CHAT_IMAGE_TTL
	store.CHAT_IMAGE_TTL = 10 * time.Millisecond
	t.Cleanup(func() {
		store.ConfigureChatImage("", "")
		store.CHAT_IMAGE_TTL = previousTTL
		producer.ResetPublisherForTest()
	})
	producer.SetPublisherForTest(&chatRoutesPublisher{})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	imagePayload := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	resp, data := doMultipartImageRequest(
		t,
		fmt.Sprintf("/chat/conversations/%s/image", conv.ID),
		authToken(t, "alice@example.com", "alice"),
		imagePayload,
		"photo.png",
		"image/png",
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var created chatMessageResponse
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode created image: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	resp, data = doJSONRequest(
		t,
		nethttp.MethodGet,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/image", conv.ID, created.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusGone {
		t.Fatalf("expected 410 on expired image, got %d: %s", resp.StatusCode, string(data))
	}

	entries, err := os.ReadDir(imageDir)
	if err != nil {
		t.Fatalf("read image dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected expired image file to be removed, got %d files", len(entries))
	}
}

func TestPostChatRead(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err = store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}
	second, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "later")
	if err != nil {
		t.Fatalf("add message 2: %v", err)
	}
	third, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "last")
	if err != nil {
		t.Fatalf("add message 3: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/read", conv.ID), authToken(t, "alice@example.com", "alice"), map[string]string{
		"message_id": second.ID,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	members, err := store.GetChatRepository().ListConversationMembers(conv.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}

	found := false
	for _, member := range members {
		if member.Email == "alice@example.com" {
			found = true
			if member.LastReadMessageID != second.ID {
				t.Fatalf("expected last read message id %q, got %#v", second.ID, member)
			}
		}
	}
	if !found {
		t.Fatalf("alice member not found: %#v", members)
	}

	messages, err := store.GetChatRepository().ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %#v", messages)
	}
	if len(messages[0].ReadBy) != 1 || messages[0].ReadBy[0].Email != "alice@example.com" {
		t.Fatalf("expected first message to be read by alice, got %#v", messages[0])
	}
	if len(messages[1].ReadBy) != 1 || messages[1].ReadBy[0].Email != "alice@example.com" {
		t.Fatalf("expected second message to be read by alice, got %#v", messages[1])
	}
	if len(messages[2].ReadBy) != 0 {
		t.Fatalf("expected third message to remain unread, got %#v", messages[2])
	}
	if third.ID == "" {
		t.Fatalf("expected third message to exist")
	}
}

func TestPostChatReadRequiresMessageID(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err = store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/read", conv.ID), authToken(t, "alice@example.com", "alice"), map[string]string{})
	if resp.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("expected 400 without message_id, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostChatReadRequiresMembership(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "mallory", "mallory@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/read", conv.ID), authToken(t, "mallory@example.com", "mallory"), map[string]string{
		"message_id": message.ID,
	})
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected 403 for non-member read, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPatchChatMessageEditsOwnTextMessage(t *testing.T) {
	chatHTTPSetup(t)

	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)
	t.Cleanup(func() {
		producer.ResetPublisherForTest()
	})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message, err := store.GetChatRepository().AddMessage(conv.ID, "alice@example.com", "alice", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(
		t,
		nethttp.MethodPatch,
		fmt.Sprintf("/chat/conversations/%s/messages/%s", conv.ID, message.ID),
		authToken(t, "alice@example.com", "alice"),
		map[string]string{"text": "updated"},
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var updated chatMessageResponse
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("decode updated message: %v", err)
	}
	if updated.Text != "updated" || updated.EditedAt == nil {
		t.Fatalf("expected edited message payload, got %#v", updated)
	}
	if len(pub.subjects) == 0 {
		t.Fatal("expected websocket publish for edited message")
	}

	messages, err := store.GetChatRepository().ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Text != "updated" || messages[0].EditedAt == nil {
		t.Fatalf("expected edited message to persist, got %#v", messages)
	}
}

func TestDeleteChatMessageAllowsAnyParticipantAndRemovesMessage(t *testing.T) {
	chatHTTPSetup(t)

	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)
	t.Cleanup(func() {
		producer.ResetPublisherForTest()
	})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(model.ChatMember{
		Email: "alice@example.com",
		Login: "alice",
	}, model.ChatMember{
		Email: "bob@example.com",
		Login: "bob",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message, err := store.GetChatRepository().AddMessage(conv.ID, "alice@example.com", "alice", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(
		t,
		nethttp.MethodDelete,
		fmt.Sprintf("/chat/conversations/%s/messages/%s", conv.ID, message.ID),
		authToken(t, "bob@example.com", "bob"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	if len(pub.subjects) == 0 {
		t.Fatal("expected websocket publish for deleted message")
	}

	messages, err := store.GetChatRepository().ListMessages(conv.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected message to be deleted, got %#v", messages)
	}
}

func TestPutChatMessageReactionUpsertsSingleReaction(t *testing.T) {
	chatHTTPSetup(t)

	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)
	t.Cleanup(func() {
		producer.ResetPublisherForTest()
	})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message, err := store.GetChatRepository().AddMessage(conv.ID, "bob@example.com", "bob", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(
		t,
		nethttp.MethodPut,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/reaction", conv.ID, message.ID),
		authToken(t, "alice@example.com", "alice"),
		map[string]string{"emoji": "🔥"},
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var reacted chatMessageResponse
	if err := json.Unmarshal(data, &reacted); err != nil {
		t.Fatalf("decode reacted message: %v", err)
	}
	if len(reacted.Reactions) != 1 || reacted.Reactions[0].Emoji != "🔥" {
		t.Fatalf("expected one reaction, got %#v", reacted)
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodPut,
		fmt.Sprintf("/chat/conversations/%s/messages/%s/reaction", conv.ID, message.ID),
		authToken(t, "alice@example.com", "alice"),
		map[string]string{"emoji": "👍"},
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &reacted); err != nil {
		t.Fatalf("decode reacted message: %v", err)
	}
	if len(reacted.Reactions) != 1 || reacted.Reactions[0].Emoji != "👍" {
		t.Fatalf("expected reaction replacement, got %#v", reacted)
	}
	if len(pub.subjects) == 0 {
		t.Fatal("expected websocket publish for reaction update")
	}
}

func TestPutChatPinSetsPinnedMessageOnConversation(t *testing.T) {
	chatHTTPSetup(t)

	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)
	t.Cleanup(func() {
		producer.ResetPublisherForTest()
	})

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message, err := store.GetChatRepository().AddMessage(conv.ID, "alice@example.com", "alice", "hello")
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	resp, data := doJSONRequest(
		t,
		nethttp.MethodPut,
		fmt.Sprintf("/chat/conversations/%s/pin", conv.ID),
		authToken(t, "bob@example.com", "bob"),
		map[string]string{"message_id": message.ID},
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var updated chatConversationResponse
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("decode pinned conversation: %v", err)
	}
	if updated.PinnedMessageID != message.ID || updated.PinnedMessage == nil || updated.PinnedMessage.ID != message.ID {
		t.Fatalf("expected pinned conversation payload, got %#v", updated)
	}
	if len(pub.subjects) == 0 {
		t.Fatal("expected websocket publish for pin update")
	}
}

func TestGetChatSearchFindsOnlyTextMessagesAcrossVisibleChats(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "mallory", "mallory@example.com")

	visible, err := store.GetChatRepository().CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create visible conversation: %v", err)
	}
	hidden, err := store.GetChatRepository().CreateDirectConversation(
		model.ChatMember{Email: "mallory@example.com", Login: "mallory"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		t.Fatalf("create hidden conversation: %v", err)
	}
	if _, err := store.GetChatRepository().AddMessage(visible.ID, "bob@example.com", "bob", "нужно уведомление в чате"); err != nil {
		t.Fatalf("add visible text message: %v", err)
	}
	if _, err := store.GetChatRepository().AddAudioMessageWithResult(visible.ID, "alice@example.com", "alice", store.ChatAudioUpload{
		FilePath:        filepath.Join(t.TempDir(), "voice.webm"),
		MimeType:        "audio/webm",
		SizeBytes:       5,
		DurationSeconds: 1,
	}); err == nil {
		// route should ignore non-text even if it exists
	}
	if _, err := store.GetChatRepository().AddMessage(hidden.ID, "mallory@example.com", "mallory", "уведомление скрыто"); err != nil {
		t.Fatalf("add hidden text message: %v", err)
	}

	resp, data := doJSONRequest(
		t,
		nethttp.MethodGet,
		"/chat/search?q=%D1%83%D0%B2%D0%B5%D0%B4%D0%BE%D0%BC%D0%BB%D0%B5%D0%BD%D0%B8%D0%B5",
		authToken(t, "alice@example.com", "alice"),
		nil,
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var results []chatSearchResultResponse
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("decode search results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one visible text result, got %#v", results)
	}
	if results[0].ConversationID != visible.ID || !strings.Contains(results[0].Text, "уведомление") {
		t.Fatalf("unexpected search result: %#v", results[0])
	}
}

func TestPostChatGroupMutationsAllowNonMember(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "carol", "carol@example.com")
	createTestUser(t, "mallory", "mallory@example.com")

	createdResp, createdData := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com"},
	})
	if createdResp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", createdResp.StatusCode, string(createdData))
	}
	var created chatConversationResponse
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatalf("decode created group: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPatch, fmt.Sprintf("/chat/conversations/group/%s", created.ID), authToken(t, "mallory@example.com", "mallory"), map[string]any{
		"title": "hacked",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	renamed, err := store.GetChatRepository().FindConversationByID(created.ID)
	if err != nil {
		t.Fatalf("find conversation after patch: %v", err)
	}
	if renamed.Title != "hacked" {
		t.Fatalf("expected title to update, got %#v", renamed)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/group/%s/members", created.ID), authToken(t, "mallory@example.com", "mallory"), map[string]any{
		"emails": []string{"carol@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 for add members, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodDelete, fmt.Sprintf("/chat/conversations/group/%s/members", created.ID), authToken(t, "mallory@example.com", "mallory"), map[string]any{
		"emails": []string{"bob@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 for delete members, got %d: %s", resp.StatusCode, string(data))
	}

	members, err := store.GetChatRepository().ListConversationMembers(created.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	foundCarol := false
	foundBob := false
	for _, member := range members {
		if member.Email == "carol@example.com" {
			foundCarol = true
		}
		if member.Email == "bob@example.com" {
			foundBob = true
		}
	}
	if !foundCarol {
		t.Fatalf("expected carol to be added, got %#v", members)
	}
	if foundBob {
		t.Fatalf("expected bob to be removed, got %#v", members)
	}
}

func TestDeleteChatGroupMembersAllowsSelfRemovalWithout500(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	createdResp, createdData := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com"},
	})
	if createdResp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", createdResp.StatusCode, string(createdData))
	}
	var created chatConversationResponse
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatalf("decode created group: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodDelete, fmt.Sprintf("/chat/conversations/group/%s/members", created.ID), authToken(t, "bob@example.com", "bob"), map[string]any{
		"emails": []string{"bob@example.com"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on self-removal, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestDeleteChatGroupConversation(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	createdResp, createdData := doJSONRequest(t, nethttp.MethodPost, "/chat/conversations/group", authToken(t, "alice@example.com", "alice"), map[string]any{
		"title":         "Team chat",
		"member_emails": []string{"bob@example.com"},
	})
	if createdResp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", createdResp.StatusCode, string(createdData))
	}
	var created chatConversationResponse
	if err := json.Unmarshal(createdData, &created); err != nil {
		t.Fatalf("decode created group: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodDelete, fmt.Sprintf("/chat/conversations/group/%s", created.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on group delete, got %d: %s", resp.StatusCode, string(data))
	}

	if _, err := store.GetChatRepository().FindConversationByID(created.ID); err == nil {
		t.Fatal("expected group conversation to be deleted")
	}
}

func TestChatCallLifecycleRoutes(t *testing.T) {
	chatHTTPSetup(t)

	pub := &chatRoutesPublisher{}
	producer.SetPublisherForTest(pub)
	t.Cleanup(producer.ResetPublisherForTest)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")

	conv, err := store.GetChatRepository().CreateGroupConversation("Team", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/calls", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on call start, got %d: %s", resp.StatusCode, string(data))
	}

	var started chatCallResponse
	if err := json.Unmarshal(data, &started); err != nil {
		t.Fatalf("decode start call: %v", err)
	}
	if started.ConversationID != conv.ID || len(started.Participants) != 1 || started.Participants[0].Email != "alice@example.com" {
		t.Fatalf("unexpected started call payload: %#v", started)
	}
	if len(pub.subjects) == 0 || pub.subjects[0] != event.ChatEventCallStarted {
		t.Fatalf("expected call started publish, got %#v", pub.subjects)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/call", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on get current call, got %d: %s", resp.StatusCode, string(data))
	}

	var current chatCallResponse
	if err := json.Unmarshal(data, &current); err != nil {
		t.Fatalf("decode current call: %v", err)
	}
	if current.ID != started.ID || current.MessageID == "" {
		t.Fatalf("unexpected current call response: %#v", current)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/messages", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on messages, got %d: %s", resp.StatusCode, string(data))
	}

	var messages []chatMessageResponse
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Type != "call" || messages[0].Call == nil || !messages[0].Call.Joinable {
		t.Fatalf("expected active call message, got %#v", messages)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/calls/%s/join", conv.ID, started.ID), authToken(t, "bob@example.com", "bob"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on join, got %d: %s", resp.StatusCode, string(data))
	}

	var joined chatCallResponse
	if err := json.Unmarshal(data, &joined); err != nil {
		t.Fatalf("decode joined call: %v", err)
	}
	if len(joined.Participants) != 2 {
		t.Fatalf("expected 2 participants after join, got %#v", joined)
	}

	resp, data = doJSONRequest(
		t,
		nethttp.MethodPut,
		fmt.Sprintf("/chat/conversations/%s/calls/%s/mute", conv.ID, started.ID),
		authToken(t, "bob@example.com", "bob"),
		map[string]any{"muted": true},
	)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on mute, got %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &joined); err != nil {
		t.Fatalf("decode muted call: %v", err)
	}
	if len(joined.Participants) != 2 || !joined.Participants[1].Muted {
		t.Fatalf("expected bob muted in response, got %#v", joined)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/calls/%s/leave", conv.ID, started.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on first leave, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/calls/%s/leave", conv.ID, started.ID), authToken(t, "bob@example.com", "bob"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on last leave, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/call", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 with null after call ended, got %d: %s", resp.StatusCode, string(data))
	}
	if strings.TrimSpace(string(data)) != "null" {
		t.Fatalf("expected null after call ended, got %s", string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, fmt.Sprintf("/chat/conversations/%s/messages", conv.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on messages after end, got %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode messages after end: %v", err)
	}
	if len(messages) != 1 || messages[0].Call == nil || messages[0].Call.Joinable || messages[0].Call.EndedAt == nil {
		t.Fatalf("expected ended call message, got %#v", messages)
	}
}

func TestGetChatCallConfig(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	if err := os.Setenv("CHAT_WEBRTC_STUN_URLS", "stun:stun.l.google.com:19302,stun:global.stun.twilio.com:3478"); err != nil {
		t.Fatalf("set stun env: %v", err)
	}
	if err := os.Setenv("CHAT_WEBRTC_TURN_URLS", "turn:turn.example.com:3478"); err != nil {
		t.Fatalf("set turn env: %v", err)
	}
	if err := os.Setenv("CHAT_WEBRTC_TURN_USERNAME", "demo-user"); err != nil {
		t.Fatalf("set turn user env: %v", err)
	}
	if err := os.Setenv("CHAT_WEBRTC_TURN_CREDENTIAL", "demo-pass"); err != nil {
		t.Fatalf("set turn credential env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("CHAT_WEBRTC_STUN_URLS")
		_ = os.Unsetenv("CHAT_WEBRTC_TURN_URLS")
		_ = os.Unsetenv("CHAT_WEBRTC_TURN_USERNAME")
		_ = os.Unsetenv("CHAT_WEBRTC_TURN_CREDENTIAL")
	})

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/chat/calls/config", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on call config, got %d: %s", resp.StatusCode, string(data))
	}

	var config chatCallConfigResponse
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode call config: %v", err)
	}
	if len(config.IceServers) != 2 {
		t.Fatalf("expected 2 ice servers, got %#v", config)
	}
}

func TestChatCallRouteRejectsSecondActiveCallForUser(t *testing.T) {
	chatHTTPSetup(t)

	producer.SetPublisherForTest(&chatRoutesPublisher{})
	t.Cleanup(producer.ResetPublisherForTest)

	createTestUser(t, "alice", "alice@example.com")
	createTestUser(t, "bob", "bob@example.com")
	createTestUser(t, "zoe", "zoe@example.com")

	first, err := store.GetChatRepository().CreateGroupConversation("One", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "bob@example.com", Login: "bob"},
	})
	if err != nil {
		t.Fatalf("create first conversation: %v", err)
	}
	second, err := store.GetChatRepository().CreateGroupConversation("Two", []model.ChatMember{
		{Email: "alice@example.com", Login: "alice"},
		{Email: "zoe@example.com", Login: "zoe"},
	})
	if err != nil {
		t.Fatalf("create second conversation: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/calls", first.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on first call, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, fmt.Sprintf("/chat/conversations/%s/calls", second.ID), authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("expected 400 on second active call, got %d: %s", resp.StatusCode, string(data))
	}
}
