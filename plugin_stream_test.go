package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// drainStream collects all chunks until both channels close or a timeout
// elapses. Returns the concatenated content and any error produced.
func drainStream(t *testing.T, chunks <-chan string, errs <-chan error) (string, error) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	var sb strings.Builder
	var streamErr error
	chunksOpen, errsOpen := true, true
	for chunksOpen || errsOpen {
		select {
		case d, ok := <-chunks:
			if !ok {
				chunksOpen = false
				chunks = nil
				continue
			}
			sb.WriteString(d)
		case e, ok := <-errs:
			if !ok {
				errsOpen = false
				errs = nil
				continue
			}
			if e != nil {
				streamErr = e
			}
		case <-deadline:
			t.Fatal("drainStream: timed out")
		}
	}
	return sb.String(), streamErr
}

// sseServer returns an httptest.Server that writes the given frames as
// text/event-stream and flushes after each. If the test wants to assert on
// request shape (auth, model, etc.) it should use an inline handler instead.
func sseServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("sseServer: ResponseWriter is not a Flusher")
		}
		for _, f := range frames {
			fmt.Fprint(w, f)
			flusher.Flush()
		}
	}))
}

func TestStreamChatCompletion_MultiChunk(t *testing.T) {
	server := sseServer(t, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n",
		"data: [DONE]\n\n",
	})
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "Hello world" {
		t.Errorf("got %q, want %q", got, "Hello world")
	}
}
