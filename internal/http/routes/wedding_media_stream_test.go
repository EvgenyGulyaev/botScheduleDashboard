package routes

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-www/silverlining"
)

type weddingStreamReader struct{ io.Reader }

func TestWeddingMediaWriterStreamsCompleteResponseOverSilverlining(t *testing.T) {
	payload := bytes.Repeat([]byte("wedding-video-range-"), 10000)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	finished := make(chan error, 1)
	server := silverlining.Server{Handler: func(ctx *silverlining.Context) {
		w := newWeddingMediaWriter(ctx)
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(206)
		_, err := io.Copy(w, weddingStreamReader{bytes.NewReader(payload)})
		finished <- err
	}}
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			server.ServeConn(conn)
		}
	}()
	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://"+ln.Addr().String()+"/content", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal("truncated stream", len(body), err)
	}
	if !bytes.Equal(body, payload) || res.StatusCode != 206 {
		t.Fatal("stream changed", len(body), res.StatusCode)
	}
	if err := <-finished; err != nil {
		t.Fatal("stream copy", err)
	}
}

func TestWeddingMediaRequestReadsStagedLargeUploadOverSilverlining(t *testing.T) {
	for _, size := range []int{90 * 1024, 8 * 1024 * 1024} {
		t.Run(strconv.Itoa(size), func(t *testing.T) { testWeddingMediaRequestUpload(t, size) })
	}
}

func testWeddingMediaRequestUpload(t *testing.T, size int) {
	t.Helper()
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i*31 + (i >> 8))
	}
	upstreamCalls := make(chan []byte, 1)
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Range") != fmt.Sprintf("bytes 0-%d/%d", len(payload)-1, len(payload)) {
			http.Error(w, "upload Content-Range changed while reading body", 400)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read uploaded bytes", 400)
			return
		}
		upstreamCalls <- body
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"complete":true}`)
	}))
	defer google.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	readResult := make(chan error, 1)
	server := silverlining.Server{ReadTimeout: 3 * time.Second, Handler: func(ctx *silverlining.Context) {
		req := weddingMediaRequest(ctx, "PUT")
		defer ctx.CloseBodyReader()
		body, err := io.ReadAll(req.Body)
		readResult <- err
		if err != nil {
			_ = ctx.WriteFullBodyString(400, "incomplete body")
			return
		}
		forward, _ := http.NewRequest("PUT", google.URL+"/upload/session", bytes.NewReader(body))
		forward.Header.Set("Content-Range", req.Header.Get("Content-Range"))
		res, err := google.Client().Do(forward)
		if err != nil {
			_ = ctx.WriteFullBodyString(503, "upstream failed")
			return
		}
		defer res.Body.Close()
		w := newWeddingMediaWriter(ctx)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	}}
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			server.ServeConn(conn)
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = fmt.Fprintf(conn, "PUT /wedding/media/uploads/session HTTP/1.1\r\nHost: wedding.test\r\nOrigin: https://wedding.test\r\nContent-Type: image/webp\r\nContent-Range: bytes 0-%d/%d\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", len(payload)-1, len(payload), len(payload))
	if err != nil {
		t.Fatal(err)
	}
	// Network packets may split anywhere; use deliberately unequal body writes.
	for offset := 0; offset < len(payload); {
		length := min(3001, len(payload)-offset)
		if _, err = conn.Write(payload[offset : offset+length]); err != nil {
			t.Fatal(err)
		}
		offset += length
		if size <= 100*1024 {
			time.Sleep(time.Millisecond)
		}
	}
	res, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "PUT"})
	if err != nil {
		t.Fatal("upload response stalled", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("upload status %d; body reader error: %v", res.StatusCode, <-readResult)
	}
	if err := <-readResult; err != nil {
		t.Fatal("body read", err)
	}
	select {
	case actual := <-upstreamCalls:
		if sha256.Sum256(actual) != sha256.Sum256(payload) {
			t.Fatalf("upload reordered or truncated: got %d bytes, want %d", len(actual), len(payload))
		}
	default:
		t.Fatal("Google upload never received")
	}
	body, err := io.ReadAll(res.Body)
	if err != nil || string(body) != `{"complete":true}` {
		t.Fatal("completion", string(body), err)
	}
}
