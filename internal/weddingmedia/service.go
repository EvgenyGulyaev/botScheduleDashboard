// Package weddingmedia keeps Drive credentials and resumable session URLs on the server.
package weddingmedia

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

const MaxFileSize int64 = 1 << 30
const ChunkSize int64 = 8 << 20
const maxSessions = 2048
const cookieName = "wedding_guest"
const sessionTTL = 24 * time.Hour
const metadataFields = "id,name,mimeType,size,createdTime,hasThumbnail,thumbnailLink,parents,trashed"

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,200}$`)
var byteRange = regexp.MustCompile(`^bytes=([0-9]+-[0-9]*|-[0-9]+)$`)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	FolderID     string `json:"folder_id"`
	CookieKey    string `json:"cookie_key"`
	Origin       string `json:"origin"`
}

// SaveConfig atomically replaces a protected configuration without printing secrets.
func SaveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".wedding-drive-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func Load(path string) (*Service, error) {
	if path == "" {
		return newService(Config{}, "", nil, "https://www.googleapis.com", "https://oauth2.googleapis.com/token")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("Drive config must have mode 0600")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if !validID.MatchString(cfg.FolderID) || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" || len(cfg.CookieKey) < 64 {
		return nil, errors.New("incomplete Drive config")
	}
	if origin := os.Getenv("WEDDING_MEDIA_ORIGIN"); origin != "" {
		cfg.Origin = origin
	}
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("Drive origin must be canonical https origin without trailing slash")
	}
	return newService(cfg, path, nil, "https://www.googleapis.com", "https://oauth2.googleapis.com/token")
}

type Service struct {
	Settings                      func() (enabled bool, version string, err error)
	cfg                           Config
	configPath, baseURL, tokenURL string
	client                        *http.Client
	db                            *bbolt.DB
	mu                            sync.Mutex
	tokenMu                       sync.Mutex
	accessToken                   string
	refreshToken                  string
	tokenExpiry                   time.Time
	sessions                      map[string]*upload
	inFlight                      chan struct{}
	creationWindow                time.Time
	creationCount                 int
	cacheMu                       sync.Mutex
	listCache                     map[string]cachedGallery
	thumbnailCache                map[string]cachedThumbnail
	summaryCache                  cachedSummary
	summaryLoading                chan struct{}
	cacheGeneration               uint64
}
type cachedSummary struct {
	Total   int
	Expires time.Time
}
type galleryPage struct {
	Items      []Media `json:"items"`
	NextCursor string  `json:"next_cursor"`
}
type cachedGallery struct {
	Page    galleryPage
	Expires time.Time
}
type cachedThumbnail struct {
	Data     []byte
	MIMEType string
	Expires  time.Time
}
type Media struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	CreatedTime  string `json:"created_time"`
	HasThumbnail bool   `json:"has_thumbnail"`
}
type driveFile struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	MIMEType      string   `json:"mimeType"`
	Size          string   `json:"size"`
	CreatedTime   string   `json:"createdTime"`
	HasThumbnail  bool     `json:"hasThumbnail"`
	ThumbnailLink string   `json:"thumbnailLink"`
	Parents       []string `json:"parents"`
	Trashed       bool     `json:"trashed"`
}

func (f driveFile) media() Media {
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	return Media{f.ID, f.Name, f.MIMEType, size, f.CreatedTime, f.HasThumbnail}
}

type UploadState struct {
	ID       string `json:"id"`
	Offset   int64  `json:"offset"`
	Complete bool   `json:"complete"`
	File     *Media `json:"file,omitempty"`
}
type upload struct {
	mu sync.Mutex
	UploadState
	Owner, URL, Name, MIMEType string
	Size                       int64
	Expires                    time.Time
	Sniffed                    bool
	FolderID                   string
}
type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string           { return e.message }
func fail(status int, message string) error { return &apiError{status, message} }

func newService(cfg Config, path string, client *http.Client, base, token string) (*Service, error) {
	if cfg.CookieKey == "" {
		cfg.CookieKey = randomID() + randomID()
	}
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	s := &Service{cfg: cfg, configPath: path, client: &copyClient, baseURL: base, tokenURL: token, refreshToken: cfg.RefreshToken, sessions: map[string]*upload{}, inFlight: make(chan struct{}, 8)}
	s.listCache = map[string]cachedGallery{}
	s.thumbnailCache = map[string]cachedThumbnail{}
	if path != "" {
		if info, err := os.Stat(path + ".sessions.db"); err == nil && info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("Drive upload database must have mode 0600")
		}
		db, err := bbolt.Open(path+".sessions.db", 0600, &bbolt.Options{Timeout: time.Second})
		if err != nil {
			return nil, err
		}
		s.db = db
		err = db.Update(func(tx *bbolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists([]byte("uploads"))
			if err != nil {
				return err
			}
			return b.ForEach(func(k, v []byte) error {
				var u upload
				if json.Unmarshal(v, &u) == nil && u.FolderID == cfg.FolderID && u.Expires.After(time.Now()) && len(s.sessions) < maxSessions {
					s.sessions[u.ID] = &u
				}
				return nil
			})
		})
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}
func (s *Service) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
func randomID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func (s *Service) connected() bool { return s.cfg.RefreshToken != "" && s.cfg.FolderID != "" }
func (s *Service) settings() (bool, string, error) {
	if s.Settings != nil {
		return s.Settings()
	}
	return false, "", nil
}

type guestClaim struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Expires int64  `json:"expires"`
}

func (s *Service) signature(data string) string {
	h := hmac.New(sha256.New, []byte(s.cfg.CookieKey))
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Service) claim(r *http.Request) (guestClaim, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || len(c.Value) > 2048 {
		return guestClaim{}, false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(s.signature(parts[0]))) {
		return guestClaim{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	var g guestClaim
	if err != nil || json.Unmarshal(data, &g) != nil || g.Expires < time.Now().Unix() || !validID.MatchString(g.ID) {
		return guestClaim{}, false
	}
	return g, true
}

// Authorize is called only after the invitation repository verified the code.
func (s *Service) Authorize(w http.ResponseWriter, r *http.Request, version string) {
	g, ok := s.claim(r)
	if !ok {
		g.ID = randomID()
	}
	g.Version = version
	g.Expires = time.Now().Add(30 * 24 * time.Hour).Unix()
	data, _ := json.Marshal(g)
	payload := base64.RawURLEncoding.EncodeToString(data)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: payload + "." + s.signature(payload), Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.cfg.Origin, "https://") || r.URL.Scheme != "http", SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
}

// CheckOrigin also protects the existing access-code endpoint when Drive is disconnected.
func (s *Service) CheckOrigin(r *http.Request) bool {
	expected := s.cfg.Origin
	if expected == "" {
		expected = r.URL.Scheme + "://" + r.Host
	}
	return r.Header.Get("Origin") == expected
}

// Before Drive is configured, retain legacy verification behind proxies that omit
// X-Forwarded-Proto. Explicit cross-site origins are still rejected.
func (s *Service) CheckVerificationOrigin(r *http.Request) bool {
	if s.connected() {
		return s.CheckOrigin(r)
	}
	if r.Header.Get("Origin") == "" {
		return true
	}
	u, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host == r.Host && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.User == nil
}
func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func errorResponse(w http.ResponseWriter, err error) {
	status := 503
	message := "Google Drive is temporarily unavailable"
	var e *apiError
	if errors.As(err, &e) {
		status = e.status
		message = e.message
	}
	jsonResponse(w, status, map[string]string{"message": message})
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" && r.Method != "HEAD" && !s.CheckOrigin(r) {
		errorResponse(w, fail(403, "invalid request origin"))
		return
	}
	enabled, version, err := s.settings()
	if err != nil {
		errorResponse(w, fail(503, "invitation settings unavailable"))
		return
	}
	g, known := s.claim(r)
	authorized := !enabled || (known && g.Version == version)
	if r.Method == "GET" && r.URL.Path == "/wedding/media/status" {
		if !known && !enabled {
			s.Authorize(w, r, version)
		}
		jsonResponse(w, 200, map[string]any{"connected": s.connected(), "guest_authorized": authorized, "max_file_size": MaxFileSize, "chunk_size": ChunkSize})
		return
	}
	if !authorized {
		errorResponse(w, fail(401, "invitation code required"))
		return
	}
	if !s.connected() {
		errorResponse(w, fail(503, "Google Drive is not connected"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	r = r.WithContext(ctx)
	path := strings.TrimPrefix(r.URL.Path, "/wedding/media")
	// Cached counts and scan waiters must leave media slots available for viewing.
	if path == "/summary" && r.Method == "GET" {
		if err = s.summary(w, r); err != nil {
			errorResponse(w, err)
		}
		return
	}
	select {
	case s.inFlight <- struct{}{}:
		defer func() { <-s.inFlight }()
	default:
		errorResponse(w, fail(429, "too many active media requests"))
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if path == "" && r.Method == "GET" {
		err = s.list(w, r)
	} else if path == "/uploads" && r.Method == "POST" {
		if !known {
			err = fail(401, "open media status first")
		} else {
			err = s.start(w, r, g.ID)
		}
	} else if len(parts) == 2 && parts[0] == "uploads" && (r.Method == "GET" || r.Method == "PUT") {
		if !known {
			err = fail(401, "guest session required")
		} else {
			err = s.resume(w, r, parts[1], g.ID)
		}
	} else if validID.MatchString(parts[0]) && r.Method == "GET" && ((len(parts) == 2 && (parts[1] == "content" || parts[1] == "thumbnail")) || len(parts) == 1) {
		err = s.content(w, r, parts)
	} else {
		err = fail(404, "media route not found")
	}
	if err != nil {
		errorResponse(w, err)
	}
}

var mediaMIMETypes = []string{"image/jpeg", "image/png", "image/gif", "image/webp", "image/heic", "image/heif", "image/avif", "image/bmp", "image/tiff", "video/mp4", "video/quicktime", "video/x-m4v", "video/webm", "video/3gpp", "video/3gpp2", "video/mpeg", "video/x-msvideo", "video/x-matroska"}

func allowedMIME(m string) bool {
	for _, allowed := range mediaMIMETypes {
		if m == allowed {
			return true
		}
	}
	return false
}
func matchesMedia(data []byte, m string) bool {
	detected := http.DetectContentType(data)
	if detected == m && allowedMIME(m) {
		return true
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		brands := string(data[8:min(len(data), 128)])
		switch m {
		case "image/heic":
			return strings.Contains(brands, "heic") || strings.Contains(brands, "heix")
		case "image/heif":
			return strings.Contains(brands, "mif1") || strings.Contains(brands, "heic") || strings.Contains(brands, "heix")
		case "image/avif":
			return strings.Contains(brands, "avif") || strings.Contains(brands, "avis")
		case "video/quicktime":
			return strings.Contains(brands, "qt  ")
		case "video/mp4", "video/x-m4v":
			return strings.Contains(brands, "mp4") || strings.Contains(brands, "isom") || strings.Contains(brands, "iso2") || strings.Contains(brands, "M4V ") || strings.Contains(brands, "avc1")
		case "video/3gpp", "video/3gpp2":
			return strings.Contains(brands, "3gp") || strings.Contains(brands, "3g2")
		}
	}
	if m == "image/tiff" && len(data) >= 4 {
		return bytes.Equal(data[:4], []byte{'I', 'I', 42, 0}) || bytes.Equal(data[:4], []byte{'M', 'M', 0, 42})
	}
	if m == "video/x-matroska" && len(data) >= 4 {
		return bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) && bytes.Contains(data[:min(len(data), 4096)], []byte("matroska"))
	}
	return false
}

func (s *Service) token(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.accessToken != "" && time.Until(s.tokenExpiry) > time.Minute {
		return s.accessToken, nil
	}
	form := url.Values{"client_id": {s.cfg.ClientID}, "client_secret": {s.cfg.ClientSecret}, "refresh_token": {s.refreshToken}, "grant_type": {"refresh_token"}}
	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fail(503, "Google Drive needs reconnection")
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err = decode(res.Body, &out); err != nil || out.AccessToken == "" {
		return "", fail(503, "invalid Google authorization response")
	}
	if out.RefreshToken != "" && out.RefreshToken != s.refreshToken {
		cfg := s.cfg
		cfg.RefreshToken = out.RefreshToken
		if s.configPath != "" {
			if err = SaveConfig(s.configPath, cfg); err != nil {
				return "", err
			}
		}
		s.refreshToken = out.RefreshToken
	}
	s.accessToken = out.AccessToken
	s.tokenExpiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return s.accessToken, nil
}
func (s *Service) do(r *http.Request) (*http.Response, error) {
	token, err := s.token(r.Context())
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	res, err := s.client.Do(r)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 || res.StatusCode == 301 || res.StatusCode == 302 || res.StatusCode == 307 {
		defer res.Body.Close()
		status := 503
		message := "Google Drive is temporarily unavailable"
		if res.StatusCode == 404 {
			status = 404
			message = "media or upload expired"
		}
		if res.StatusCode == 429 {
			status = 429
			message = "Google Drive quota exceeded; try again later"
		}
		if res.StatusCode == 416 {
			status = 416
			message = "requested range is outside this file"
		}
		if res.StatusCode == 403 {
			var problem struct {
				Error struct {
					Errors []struct {
						Reason string `json:"reason"`
					} `json:"errors"`
				} `json:"error"`
			}
			if decode(res.Body, &problem) == nil {
				for _, e := range problem.Error.Errors {
					switch e.Reason {
					case "storageQuotaExceeded":
						message = "Google Drive has no free storage"
					case "downloadQuotaExceeded", "rateLimitExceeded", "userRateLimitExceeded":
						status = 429
						message = "Google Drive quota exceeded; try again later"
					}
				}
			}
		}
		return nil, fail(status, message)
	}
	return res, nil
}
func decode(r io.Reader, out any) error {
	data, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("Google metadata too large")
	}
	return json.Unmarshal(data, out)
}
func (s *Service) get(ctx context.Context, target string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	return s.do(r)
}
func (s *Service) metadata(ctx context.Context, id string) (driveFile, error) {
	res, err := s.get(ctx, s.baseURL+"/drive/v3/files/"+url.PathEscape(id)+"?fields="+url.QueryEscape(metadataFields))
	if err != nil {
		return driveFile{}, err
	}
	defer res.Body.Close()
	var f driveFile
	if err = decode(res.Body, &f); err != nil {
		return f, err
	}
	if f.Trashed || !allowedMIME(f.MIMEType) {
		return f, fail(404, "media not found")
	}
	for _, p := range f.Parents {
		if p == s.cfg.FolderID {
			return f, nil
		}
	}
	return f, fail(404, "media not found")
}
func (s *Service) list(w http.ResponseWriter, r *http.Request) error {
	cursor := r.URL.Query().Get("cursor")
	if len(cursor) > 2048 {
		return fail(400, "invalid gallery cursor")
	}
	s.cacheMu.Lock()
	cached, ok := s.listCache[cursor]
	generation := s.cacheGeneration
	s.cacheMu.Unlock()
	if ok && time.Now().Before(cached.Expires) {
		jsonResponse(w, 200, cached.Page)
		return nil
	}
	q := url.Values{"q": {s.mediaQuery()}, "pageSize": {"20"}, "orderBy": {"createdTime desc"}, "fields": {"nextPageToken,files(" + metadataFields + ")"}}
	if cursor != "" {
		q.Set("pageToken", cursor)
	}
	res, err := s.get(r.Context(), s.baseURL+"/drive/v3/files?"+q.Encode())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var page struct {
		Files []driveFile `json:"files"`
		Next  string      `json:"nextPageToken"`
	}
	if err = decode(res.Body, &page); err != nil {
		return err
	}
	if len(page.Next) > 2048 || (page.Next != "" && page.Next == cursor) {
		return fail(503, "invalid Google Drive pagination")
	}
	items := []Media{}
	for _, f := range page.Files {
		if allowedMIME(f.MIMEType) && !f.Trashed {
			for _, p := range f.Parents {
				if p == s.cfg.FolderID {
					items = append(items, f.media())
					break
				}
			}
		}
	}
	result := galleryPage{Items: items, NextCursor: page.Next}
	s.cacheMu.Lock()
	if generation == s.cacheGeneration {
		if len(s.listCache) >= 32 {
			s.listCache = map[string]cachedGallery{}
		}
		s.listCache[cursor] = cachedGallery{Page: result, Expires: time.Now().Add(10 * time.Second)}
	}
	s.cacheMu.Unlock()
	jsonResponse(w, 200, result)
	return nil
}

func (s *Service) mediaQuery() string {
	filters := make([]string, len(mediaMIMETypes))
	for i, mime := range mediaMIMETypes {
		filters[i] = "mimeType = '" + mime + "'"
	}
	return "'" + s.cfg.FolderID + "' in parents and trashed = false and (" + strings.Join(filters, " or ") + ")"
}

func (s *Service) summary(w http.ResponseWriter, r *http.Request) error {
	for {
		s.cacheMu.Lock()
		if time.Now().Before(s.summaryCache.Expires) {
			total := s.summaryCache.Total
			s.cacheMu.Unlock()
			jsonResponse(w, 200, map[string]int{"total": total})
			return nil
		}
		if loading := s.summaryLoading; loading != nil {
			s.cacheMu.Unlock()
			select {
			case <-loading:
				continue
			case <-r.Context().Done():
				return r.Context().Err()
			}
		}
		loading := make(chan struct{})
		s.summaryLoading = loading
		generation := s.cacheGeneration
		s.cacheMu.Unlock()

		var total int
		var err error
		select {
		case s.inFlight <- struct{}{}:
			total, err = s.countMedia(r.Context())
			<-s.inFlight
		default:
			err = fail(429, "too many active media requests")
		}
		s.cacheMu.Lock()
		if err == nil && generation == s.cacheGeneration {
			s.summaryCache = cachedSummary{Total: total, Expires: time.Now().Add(30 * time.Second)}
		}
		s.summaryLoading = nil
		close(loading)
		s.cacheMu.Unlock()
		if err != nil {
			return err
		}
		jsonResponse(w, 200, map[string]int{"total": total})
		return nil
	}
}

func (s *Service) countMedia(ctx context.Context) (int, error) {
	q := url.Values{"q": {s.mediaQuery()}, "pageSize": {"1000"}, "fields": {"nextPageToken,incompleteSearch,files(id,mimeType,parents,trashed)"}}
	seen := map[string]bool{"": true}
	total := 0
	// Bound malformed upstream pagination as well as the existing request deadline.
	for pages := 0; pages < 10000; pages++ {
		res, err := s.get(ctx, s.baseURL+"/drive/v3/files?"+q.Encode())
		if err != nil {
			return 0, err
		}
		var page struct {
			Files      []driveFile `json:"files"`
			Next       string      `json:"nextPageToken"`
			Incomplete bool        `json:"incompleteSearch"`
		}
		err = decode(res.Body, &page)
		res.Body.Close()
		if err != nil {
			return 0, err
		}
		if page.Incomplete {
			return 0, fail(503, "Google Drive media count is incomplete")
		}
		for _, file := range page.Files {
			if !file.Trashed && allowedMIME(file.MIMEType) {
				for _, parent := range file.Parents {
					if parent == s.cfg.FolderID {
						total++
						break
					}
				}
			}
		}
		if page.Next == "" {
			return total, nil
		}
		if len(page.Next) > 2048 || seen[page.Next] {
			return 0, fail(503, "invalid Google Drive pagination")
		}
		seen[page.Next] = true
		q.Set("pageToken", page.Next)
	}
	return 0, fail(503, "Google Drive media count exceeded page limit")
}

func (s *Service) content(w http.ResponseWriter, r *http.Request, parts []string) error {
	f, err := s.metadata(r.Context(), parts[0])
	if err != nil {
		return err
	}
	if len(parts) == 1 {
		jsonResponse(w, 200, f.media())
		return nil
	}
	target := s.baseURL + "/drive/v3/files/" + url.PathEscape(parts[0]) + "?alt=media"
	thumbnail := parts[1] == "thumbnail"
	if thumbnail {
		s.cacheMu.Lock()
		cached, ok := s.thumbnailCache[f.ID]
		s.cacheMu.Unlock()
		if ok && time.Now().Before(cached.Expires) {
			w.Header().Set("Content-Type", cached.MIMEType)
			w.Header().Set("Content-Length", strconv.Itoa(len(cached.Data)))
			w.WriteHeader(200)
			_, _ = w.Write(cached.Data)
			return nil
		}
		u, err := url.Parse(f.ThumbnailLink)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !strings.HasSuffix(u.Hostname(), ".googleusercontent.com") {
			return fail(404, "thumbnail unavailable")
		}
		target = u.String()
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", target, nil)
	if err != nil {
		return err
	}
	if value := r.Header.Get("Range"); value != "" && !thumbnail {
		if !byteRange.MatchString(value) || len(value) > 100 {
			return fail(416, "invalid byte range")
		}
		req.Header.Set("Range", value)
	}
	res, err := s.do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		return fail(503, "invalid Drive content response")
	}
	if thumbnail {
		ct := res.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "image/") || strings.Contains(ct, "svg") {
			return fail(415, "invalid thumbnail")
		}
		w.Header().Set("Content-Type", ct)
		if res.ContentLength > 0 && res.ContentLength <= 512*1024 {
			data, err := io.ReadAll(io.LimitReader(res.Body, 512*1024+1))
			if err != nil || len(data) > 512*1024 {
				return fail(503, "invalid thumbnail response")
			}
			s.cacheMu.Lock()
			if len(s.thumbnailCache) >= 16 {
				s.thumbnailCache = map[string]cachedThumbnail{}
			}
			s.thumbnailCache[f.ID] = cachedThumbnail{Data: data, MIMEType: ct, Expires: time.Now().Add(time.Minute)}
			s.cacheMu.Unlock()
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(200)
			_, _ = w.Write(data)
			return nil
		}
	} else {
		w.Header().Set("Content-Type", f.MIMEType)
	}
	for _, h := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := res.Header.Get(h); value != "" {
			w.Header().Set(h, value)
		}
	}
	if !thumbnail {
		disposition := "inline"
		if r.URL.Query().Get("download") == "1" {
			disposition = "attachment"
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Name}))
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
	return nil
}

func (s *Service) persist(u *upload) error {
	if s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("uploads"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var old upload
			if json.Unmarshal(v, &old) != nil || old.Expires.Before(time.Now()) {
				if err := c.Delete(); err != nil {
					return err
				}
			}
		}
		data, err := json.Marshal(u)
		if err != nil {
			return err
		}
		return b.Put([]byte(u.ID), data)
	})
}
func (s *Service) start(w http.ResponseWriter, r *http.Request, owner string) error {
	var in struct {
		Name     string `json:"name"`
		MIMEType string `json:"mime_type"`
		Size     int64  `json:"size"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4097)).Decode(&in); err != nil {
		return fail(400, "invalid upload metadata")
	}
	if in.Size > MaxFileSize {
		return fail(413, "file exceeds 1 GiB")
	}
	if in.Size <= 0 || len(in.Name) == 0 || len(in.Name) > 240 || strings.ContainsAny(in.Name, "/\\\r\n\x00") || in.Name == "." || in.Name == ".." {
		return fail(400, "invalid file name or size")
	}
	if !allowedMIME(in.MIMEType) {
		return fail(415, "unsupported media type")
	}
	s.mu.Lock()
	now := time.Now()
	if now.Sub(s.creationWindow) >= time.Minute {
		s.creationWindow = now
		s.creationCount = 0
	}
	if s.creationCount >= 60 {
		s.mu.Unlock()
		return fail(429, "too many new uploads; try again in a minute")
	}
	ownerCount := 0
	for id, u := range s.sessions {
		if now.After(u.Expires) {
			delete(s.sessions, id)
		} else if u.Owner == owner {
			ownerCount++
		}
	}
	if len(s.sessions) >= maxSessions || ownerCount >= 100 {
		s.mu.Unlock()
		return fail(429, "upload session limit reached; try later")
	}
	session := &upload{UploadState: UploadState{ID: randomID()}, Owner: owner, Name: in.Name, MIMEType: in.MIMEType, Size: in.Size, Expires: now.Add(sessionTTL), FolderID: s.cfg.FolderID}
	s.sessions[session.ID] = session
	s.creationCount++
	s.mu.Unlock()
	created := false
	defer func() {
		if !created {
			s.mu.Lock()
			delete(s.sessions, session.ID)
			s.mu.Unlock()
		}
	}()
	body, _ := json.Marshal(map[string]any{"name": in.Name, "mimeType": in.MIMEType, "parents": []string{s.cfg.FolderID}})
	req, err := http.NewRequestWithContext(r.Context(), "POST", s.baseURL+"/upload/drive/v3/files?uploadType=resumable&fields="+url.QueryEscape(metadataFields), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upload-Content-Type", in.MIMEType)
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(in.Size, 10))
	res, err := s.do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	target := res.Header.Get("Location")
	u, err := url.Parse(target)
	base, _ := url.Parse(s.baseURL)
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || !strings.HasPrefix(u.Path, "/upload/") {
		return fail(503, "invalid Drive upload endpoint")
	}
	session.URL = target
	if err = s.persist(session); err != nil {
		return err
	}
	created = true
	jsonResponse(w, 200, session.UploadState)
	return nil
}
func (s *Service) resume(w http.ResponseWriter, r *http.Request, id, owner string) error {
	s.mu.Lock()
	u, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok || u.Owner != owner || time.Now().After(u.Expires) {
		return fail(404, "upload session expired")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.Complete {
		jsonResponse(w, 200, u.UploadState)
		return nil
	}
	var data []byte
	contentRange := "bytes */" + strconv.FormatInt(u.Size, 10)
	if r.Method == "PUT" {
		var start, end, total int64
		var trailing string
		n, _ := fmt.Sscanf(r.Header.Get("Content-Range"), "bytes %d-%d/%d%s", &start, &end, &total, &trailing)
		if n != 3 || start < 0 || end < start || total != u.Size || end >= total || end-start+1 > ChunkSize {
			return fail(400, "invalid upload range")
		}
		if start != u.Offset {
			return fail(409, "upload offset changed; check session status")
		}
		length := end - start + 1
		if end+1 < total && length%(256*1024) != 0 {
			return fail(400, "non-final chunks must be multiples of 256 KiB")
		}
		var err error
		data, err = io.ReadAll(io.LimitReader(r.Body, ChunkSize+1))
		if err != nil || int64(len(data)) != length {
			return fail(400, "chunk length does not match range")
		}
		if start == 0 {
			if !matchesMedia(data, u.MIMEType) {
				return fail(415, "file content does not match its media type")
			}
			u.Sniffed = true
			// Save validation before sending: Drive may accept the bytes and lose its response.
			if err = s.persist(u); err != nil {
				return err
			}
		}
		contentRange = r.Header.Get("Content-Range")
	}
	req, err := http.NewRequestWithContext(r.Context(), "PUT", u.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Range", contentRange)
	if data != nil {
		req.Header.Set("Content-Type", u.MIMEType)
	}
	res, err := s.do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 308 {
		offset := int64(0)
		if value := res.Header.Get("Range"); value != "" {
			var end int64
			var extra string
			n, _ := fmt.Sscanf(value, "bytes=0-%d%s", &end, &extra)
			if n != 1 || end < 0 || end >= u.Size {
				return fail(503, "invalid Drive upload offset")
			}
			offset = end + 1
		}
		if offset < u.Offset {
			return fail(503, "Drive upload offset moved backwards")
		}
		u.Offset = offset
	} else if res.StatusCode == 200 || res.StatusCode == 201 {
		var f driveFile
		if err = decode(res.Body, &f); err != nil {
			return err
		}
		size, _ := strconv.ParseInt(f.Size, 10, 64)
		if f.ID == "" || size != u.Size || !u.Sniffed {
			return fail(503, "invalid completed upload")
		}
		m := f.media()
		u.Offset = u.Size
		u.Complete = true
		u.File = &m
		s.cacheMu.Lock()
		s.listCache = map[string]cachedGallery{}
		s.summaryCache = cachedSummary{}
		s.cacheGeneration++
		s.cacheMu.Unlock()
	} else {
		return fail(503, "unexpected Drive upload response")
	}
	if err = s.persist(u); err != nil {
		return err
	}
	jsonResponse(w, 200, u.UploadState)
	return nil
}
