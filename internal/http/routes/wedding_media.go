package routes

import (
	"botDashboard/internal/store"
	"botDashboard/internal/weddingmedia"
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-www/silverlining"
)

var mediaOnce sync.Once
var mediaService *weddingmedia.Service

// StartWeddingMediaWorker resumes persisted work independently of guest traffic.
func StartWeddingMediaWorker() func() {
	service := weddingMediaService()
	return func() {
		if err := service.Close(); err != nil {
			log.Print("wedding media: shutdown storage error")
		}
	}
}

func weddingMediaService() *weddingmedia.Service {
	mediaOnce.Do(func() {
		var err error
		mediaService, err = weddingmedia.Load(os.Getenv("WEDDING_DRIVE_CONFIG"))
		if err != nil {
			log.Print("wedding media disabled: check protected Drive configuration")
			mediaService, _ = weddingmedia.Load("")
		}
		mediaService.Settings = func() (bool, string, error) {
			settings, err := store.GetWeddingRepository().GetSettings()
			return settings.AccessCodeEnabled, settings.AccessCodeVersion, err
		}
		mediaService.StartVideoWorker()
	})
	return mediaService
}

type weddingMediaWriter struct {
	ctx     *silverlining.Context
	headers http.Header
	written bool
}

func (w *weddingMediaWriter) Header() http.Header { return w.headers }
func (w *weddingMediaWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.written = true
	for name, values := range w.headers {
		if name == "Content-Length" {
			continue
		}
		for _, value := range values {
			w.ctx.ResponseHeaders().Set(name, value)
		}
	}
	if length, err := strconv.Atoi(w.headers.Get("Content-Length")); err == nil {
		w.ctx.SetContentLength(length)
	} else {
		w.ctx.ConnectionClose()
	}
	w.ctx.WriteHeader(status)
}
func (w *weddingMediaWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.WriteHeader(200)
	}
	_, err := w.ctx.Write(p)
	if err != nil {
		return 0, err
	}
	// silverlining v1.3.3 reports only the final buffered segment after a flush.
	return len(p), nil
}

type weddingMediaBodyReader struct{ io.Reader }

func (r weddingMediaBodyReader) Read(p []byte) (int, error) {
	// silverlining v1.3.3's larger direct reads skip buffered bytes and its index.
	// Keep reads within its 8 KiB buffer so framing and byte order stay correct.
	return r.Reader.Read(p[:min(len(p), 8*1024)])
}

func weddingMediaRequest(ctx *silverlining.Context, method string) *http.Request {
	headers := http.Header{}
	for _, name := range []string{"Host", "Cookie", "Origin", "Range", "Content-Range", "Content-Type", "Content-Length", "Sec-Fetch-Site"} {
		if value, ok := ctx.RequestHeaders().Get(name); ok {
			// Get borrows silverlining's read buffer, which body reads overwrite.
			headers.Set(name, strings.Clone(value))
		}
	}
	scheme := "http"
	if isLoopbackAddr(weddingRemoteAddr(ctx)) {
		if value, _ := ctx.RequestHeaders().Get("X-Forwarded-Proto"); value == "https" {
			scheme = "https"
		}
	}
	u, _ := url.Parse(string(ctx.RawURI()))
	u.Scheme = scheme
	u.Host = headers.Get("Host")
	req := &http.Request{Method: method, URL: u, Host: u.Host, Header: headers, Body: io.NopCloser(weddingMediaBodyReader{ctx.BodyReader()}), RemoteAddr: ctx.RemoteAddr().String()}
	req.ContentLength, _ = strconv.ParseInt(headers.Get("Content-Length"), 10, 64)
	return req
}
func newWeddingMediaWriter(ctx *silverlining.Context) *weddingMediaWriter {
	return &weddingMediaWriter{ctx: ctx, headers: http.Header{}}
}
func ServeWeddingMedia(ctx *silverlining.Context) {
	method := ""
	switch ctx.Method() {
	case silverlining.MethodGET:
		method = "GET"
	case silverlining.MethodPOST:
		method = "POST"
	case silverlining.MethodPUT:
		method = "PUT"
	case silverlining.MethodDELETE:
		method = "DELETE"
	case silverlining.MethodPATCH:
		method = "PATCH"
	case silverlining.MethodOPTIONS:
		method = "OPTIONS"
	}
	req := weddingMediaRequest(ctx, method)
	defer ctx.CloseBodyReader()
	// silverlining has no request context; the deadline bounds upstream calls.
	requestContext, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	req = req.WithContext(requestContext)
	if req.ContentLength < 0 || req.ContentLength > weddingmedia.ChunkSize {
		GetError(ctx, &Error{Message: "media request exceeds chunk limit", Status: 413})
		return
	}
	weddingMediaService().ServeHTTP(newWeddingMediaWriter(ctx), req)
}
func authorizeWeddingMedia(ctx *silverlining.Context, version string) {
	w := newWeddingMediaWriter(ctx)
	weddingMediaService().Authorize(w, weddingMediaRequest(ctx, "POST"), version)
	ctx.ResponseHeaders().Set("Set-Cookie", w.Header().Get("Set-Cookie"))
}
func validWeddingMediaOrigin(ctx *silverlining.Context) bool {
	return weddingMediaService().CheckVerificationOrigin(weddingMediaRequest(ctx, "POST"))
}
