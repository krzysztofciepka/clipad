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

func TestStreamChatCompletion_MalformedFrameSkipped(t *testing.T) {
	server := sseServer(t, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n",
		"data: {not valid json}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\n",
		"data: [DONE]\n\n",
	})
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "firstsecond" {
		t.Errorf("got %q, want %q", got, "firstsecond")
	}
}

func TestStreamChatCompletion_KeepAliveSkipped(t *testing.T) {
	server := sseServer(t, []string{
		": keep-alive\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n",
		": ping\n\n",
		"\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n",
		"data: [DONE]\n\n",
	})
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "ab" {
		t.Errorf("got %q, want %q", got, "ab")
	}
}

func TestStreamChatCompletion_FallbackToBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"full response"}}]}`)
	}))
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "full response" {
		t.Errorf("got %q, want %q", got, "full response")
	}
}

func TestStreamChatCompletion_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "bad-key", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if got != "" {
		t.Errorf("got chunks %q, want none", got)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v, want substring 401", err)
	}
}

func TestStreamChatCompletion_CancelMidStream(t *testing.T) {
	requestDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestDone)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	chunks, errs := streamChatCompletion(ctx, server.URL, "k", "m", "sys", "user")

	select {
	case d := <-chunks:
		if d != "first" {
			t.Fatalf("first chunk = %q, want %q", d, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive first chunk")
	}

	cancel()

	deadline := time.After(2 * time.Second)
	chunksClosed, errsClosed := false, false
	for !chunksClosed || !errsClosed {
		select {
		case _, ok := <-chunks:
			if !ok {
				chunks = nil
				chunksClosed = true
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
				errsClosed = true
			}
		case <-deadline:
			t.Fatal("channels did not close after cancel")
		}
	}

	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server-side request did not terminate after client cancel")
	}
}
