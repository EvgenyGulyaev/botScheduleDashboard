package routes

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"

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
