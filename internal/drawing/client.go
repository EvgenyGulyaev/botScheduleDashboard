package drawing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	nethttp "net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"botDashboard/internal/model"
)

type Client struct {
	baseURL string
	token   string
	http    *nethttp.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		token:   cfg.ServiceToken,
		http:    &nethttp.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) buildRequest(ctx context.Context, method, path string, user model.UserData, body io.Reader, contentType string) (*nethttp.Request, error) {
	endpoint := c.baseURL + path
	req, err := nethttp.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Service-Token", c.token)
	if user.Email != "" {
		req.Header.Set("X-User-Email", user.Email)
	}
	if user.Login != "" {
		req.Header.Set("X-User-Login", user.Login)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

type ImageItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	MimeType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	CreatedBy string `json:"created_by"`
	UpdatedBy string `json:"updated_by"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type listResponse struct {
	Items []ImageItem `json:"items"`
}

type StampItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TextValue     string `json:"textValue"`
	HasImage      bool   `json:"hasImage"`
	ImageMimeType string `json:"imageMimeType"`
	ImageSize     int64  `json:"imageSize"`
	ImageWidth    int    `json:"imageWidth"`
	ImageHeight   int    `json:"imageHeight"`
	Priority      string `json:"priority"`
	CreatedBy     string `json:"createdBy"`
	UpdatedBy     string `json:"updatedBy"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type stampListResponse struct {
	Items []StampItem `json:"items"`
}

func (c *Client) ListImages(ctx context.Context, user model.UserData) ([]ImageItem, error) {
	req, err := c.buildRequest(ctx, nethttp.MethodGet, "/internal/drawing/images", user, nil, "")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return nil, errorFromResponse(resp)
	}
	var payload listResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func (c *Client) GetImage(ctx context.Context, user model.UserData, id string) (ImageItem, error) {
	req, err := c.buildRequest(ctx, nethttp.MethodGet, "/internal/drawing/images/"+url.PathEscape(id), user, nil, "")
	if err != nil {
		return ImageItem{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ImageItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return ImageItem{}, errorFromResponse(resp)
	}
	var item ImageItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return ImageItem{}, err
	}
	return item, nil
}

func (c *Client) DownloadImage(ctx context.Context, user model.UserData, id string) (io.ReadCloser, string, error) {
	req, err := c.buildRequest(ctx, nethttp.MethodGet, "/internal/drawing/images/"+url.PathEscape(id)+"/content", user, nil, "")
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != nethttp.StatusOK {
		defer resp.Body.Close()
		return nil, "", errorFromResponse(resp)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

type CreatePayload struct {
	Title  string
	Width  int
	Height int
	Body   io.Reader
}

func (c *Client) CreateImage(ctx context.Context, user model.UserData, payload CreatePayload) (ImageItem, error) {
	body, contentType, err := buildMultipart(payload)
	if err != nil {
		return ImageItem{}, err
	}
	req, err := c.buildRequest(ctx, nethttp.MethodPost, "/internal/drawing/images", user, body, contentType)
	if err != nil {
		return ImageItem{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ImageItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return ImageItem{}, errorFromResponse(resp)
	}
	var item ImageItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return ImageItem{}, err
	}
	return item, nil
}

func (c *Client) UpdateImage(ctx context.Context, user model.UserData, id string, payload CreatePayload) (ImageItem, error) {
	body, contentType, err := buildMultipart(payload)
	if err != nil {
		return ImageItem{}, err
	}
	req, err := c.buildRequest(ctx, nethttp.MethodPut, "/internal/drawing/images/"+url.PathEscape(id), user, body, contentType)
	if err != nil {
		return ImageItem{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ImageItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return ImageItem{}, errorFromResponse(resp)
	}
	var item ImageItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return ImageItem{}, err
	}
	return item, nil
}

func (c *Client) DeleteImage(ctx context.Context, user model.UserData, id string) error {
	req, err := c.buildRequest(ctx, nethttp.MethodDelete, "/internal/drawing/images/"+url.PathEscape(id), user, nil, "")
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusNoContent && resp.StatusCode != nethttp.StatusOK {
		return errorFromResponse(resp)
	}
	return nil
}

func (c *Client) ListStamps(ctx context.Context, user model.UserData) ([]StampItem, error) {
	req, err := c.buildRequest(ctx, nethttp.MethodGet, "/internal/drawing/stamps", user, nil, "")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return nil, errorFromResponse(resp)
	}
	var payload stampListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func (c *Client) GetStamp(ctx context.Context, user model.UserData, id string) (StampItem, error) {
	req, err := c.buildRequest(ctx, nethttp.MethodGet, "/internal/drawing/stamps/"+url.PathEscape(id), user, nil, "")
	if err != nil {
		return StampItem{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return StampItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return StampItem{}, errorFromResponse(resp)
	}
	var item StampItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return StampItem{}, err
	}
	return item, nil
}

func (c *Client) DownloadStampImage(ctx context.Context, user model.UserData, id string) (io.ReadCloser, string, error) {
	req, err := c.buildRequest(ctx, nethttp.MethodGet, "/internal/drawing/stamps/"+url.PathEscape(id)+"/content", user, nil, "")
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != nethttp.StatusOK {
		defer resp.Body.Close()
		return nil, "", errorFromResponse(resp)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

type StampPayload struct {
	Name        string
	TextValue   string
	Priority    string
	RemoveImage bool
	Body        io.Reader
	Filename    string
	MimeType    string
}

func (c *Client) CreateStamp(ctx context.Context, user model.UserData, payload StampPayload) (StampItem, error) {
	body, contentType, err := buildStampMultipart(payload)
	if err != nil {
		return StampItem{}, err
	}
	req, err := c.buildRequest(ctx, nethttp.MethodPost, "/internal/drawing/stamps", user, body, contentType)
	if err != nil {
		return StampItem{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return StampItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return StampItem{}, errorFromResponse(resp)
	}
	var item StampItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return StampItem{}, err
	}
	return item, nil
}

func (c *Client) UpdateStamp(ctx context.Context, user model.UserData, id string, payload StampPayload) (StampItem, error) {
	body, contentType, err := buildStampMultipart(payload)
	if err != nil {
		return StampItem{}, err
	}
	req, err := c.buildRequest(ctx, nethttp.MethodPut, "/internal/drawing/stamps/"+url.PathEscape(id), user, body, contentType)
	if err != nil {
		return StampItem{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return StampItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return StampItem{}, errorFromResponse(resp)
	}
	var item StampItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return StampItem{}, err
	}
	return item, nil
}

func (c *Client) DeleteStamp(ctx context.Context, user model.UserData, id string) error {
	req, err := c.buildRequest(ctx, nethttp.MethodDelete, "/internal/drawing/stamps/"+url.PathEscape(id), user, nil, "")
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusNoContent && resp.StatusCode != nethttp.StatusOK {
		return errorFromResponse(resp)
	}
	return nil
}

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return "drawing service: " + strconv.Itoa(e.Status) + " " + e.Message
}

func errorFromResponse(resp *nethttp.Response) error {
	data, _ := io.ReadAll(resp.Body)
	text := strings.TrimSpace(string(data))
	if text == "" {
		text = resp.Status
	}
	return &Error{Status: resp.StatusCode, Message: text}
}

func buildMultipart(payload CreatePayload) (io.Reader, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	metadata, err := json.Marshal(map[string]any{
		"title":  payload.Title,
		"width":  payload.Width,
		"height": payload.Height,
	})
	if err != nil {
		return nil, "", err
	}
	if err := mw.WriteField("metadata", string(metadata)); err != nil {
		return nil, "", err
	}
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="image.png"`)
	hdr.Set("Content-Type", "image/png")
	fw, err := mw.CreatePart(hdr)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(fw, payload.Body); err != nil {
		return nil, "", fmt.Errorf("copy file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}

func buildStampMultipart(payload StampPayload) (io.Reader, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	metadata, err := json.Marshal(map[string]any{
		"name":        payload.Name,
		"textValue":   payload.TextValue,
		"priority":    payload.Priority,
		"removeImage": payload.RemoveImage,
	})
	if err != nil {
		return nil, "", err
	}
	if err := mw.WriteField("metadata", string(metadata)); err != nil {
		return nil, "", err
	}
	if payload.Body != nil {
		filename := payload.Filename
		if filename == "" {
			filename = "stamp.png"
		}
		mimeType := payload.MimeType
		if mimeType == "" {
			mimeType = "image/png"
		}
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		hdr.Set("Content-Type", mimeType)
		fw, err := mw.CreatePart(hdr)
		if err != nil {
			return nil, "", err
		}
		if _, err := io.Copy(fw, payload.Body); err != nil {
			return nil, "", fmt.Errorf("copy stamp file: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}
