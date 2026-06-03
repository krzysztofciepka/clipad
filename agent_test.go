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

func TestDispatchTool_Bash(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, cites, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"echo hi"}`)
	if !ok {
		t.Errorf("ok = false, want true")
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("output %q missing 'hi'", out)
	}
	if cites != nil {
		t.Errorf("bash should produce no citations, got %v", cites)
	}
}

func TestDispatchTool_BashGuardRejects(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, _, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"sudo rm x"}`)
	if ok {
		t.Errorf("guarded command should report ok=false")
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("output %q should explain the block", out)
	}
}

func TestDispatchTool_SearchVaultNoEmbedder(t *testing.T) {
	deps := agentDeps{idx: nil}
	out, _, ok := dispatchTool(context.Background(), deps, "search_vault", `{"query":"taxes"}`)
	if ok {
		t.Errorf("search without embedder should report ok=false")
	}
	if !strings.Contains(strings.ToLower(out), "not configured") {
		t.Errorf("output %q should explain missing config", out)
	}
}

func TestDispatchTool_SearchVaultReturnsCitations(t *testing.T) {
	vault := t.TempDir()
	writeFile(t, vault, "a.md", "Taxes are due in April.\n\nUnrelated paragraph.")
	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.RebuildFile(context.Background(), vault+"/a.md"); err != nil {
		t.Fatal(err)
	}
	deps := agentDeps{idx: idx, vault: vault}
	out, cites, ok := dispatchTool(context.Background(), deps, "search_vault", `{"query":"Taxes are due in April.","k":2}`)
	if !ok {
		t.Fatalf("ok=false, out=%q", out)
	}
	if len(cites) == 0 {
		t.Errorf("expected citations, got none")
	}
	if !strings.Contains(out, "a.md") {
		t.Errorf("output %q should reference a.md", out)
	}
}

func TestDispatchTool_UnknownTool(t *testing.T) {
	out, _, ok := dispatchTool(context.Background(), agentDeps{}, "nope", `{}`)
	if ok || !strings.Contains(out, "unknown tool") {
		t.Errorf("unknown tool should report ok=false with message, got ok=%v out=%q", ok, out)
	}
}

func TestDispatchTool_BashNonZeroExit(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, _, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"echo boom; exit 1"}`)
	if ok {
		t.Errorf("non-zero exit should report ok=false")
	}
	if !strings.Contains(out, "boom") {
		t.Errorf("output %q should still contain command output", out)
	}
	if !strings.Contains(out, "exit 1") {
		t.Errorf("output %q should include the exit code", out)
	}
}

func TestDispatchTool_BashMalformedArgs(t *testing.T) {
	deps := agentDeps{vault: t.TempDir(), timeout: 5 * time.Second}
	out, _, ok := dispatchTool(context.Background(), deps, "bash", `{bad json`)
	if ok {
		t.Errorf("malformed args should report ok=false")
	}
	if !strings.Contains(out, "error") {
		t.Errorf("output %q should describe the error", out)
	}
}

func TestDispatchTool_BashDefaultsTimeoutWhenZero(t *testing.T) {
	// timeout left at zero must NOT cause an instant timeout; the command runs.
	deps := agentDeps{vault: t.TempDir()} // timeout == 0
	out, _, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"echo alive"}`)
	if !ok {
		t.Errorf("ok=false with zero timeout; expected default to apply. out=%q", out)
	}
	if !strings.Contains(out, "alive") {
		t.Errorf("output %q missing 'alive' (instant timeout?)", out)
	}
}

func TestFormatSearchResults_Empty(t *testing.T) {
	out, cites, ok := formatSearchResults(nil)
	if !ok || cites != nil {
		t.Errorf("empty results: got ok=%v cites=%v, want ok=true cites=nil", ok, cites)
	}
	if !strings.Contains(out, "No matching notes found") {
		t.Errorf("output %q should report no matches", out)
	}
}

func TestAgentSystemPrompt_MentionsVaultAndTools(t *testing.T) {
	p := agentSystemPrompt("/home/u/notes")
	for _, want := range []string{"/home/u/notes", "search_vault", "bash"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestParseSlashCommand(t *testing.T) {
	cases := map[string]slashCommand{
		"/clear":      slashClear,
		"/exit":       slashExit,
		"/model":      slashModel,
		"/help":       slashHelp,
		"/bogus":      slashUnknown,
		"not a slash": slashNone,
	}
	for in, want := range cases {
		if got := parseSlashCommand(in); got != want {
			t.Errorf("parseSlashCommand(%q) = %v, want %v", in, got, want)
		}
	}
}

// scriptedServer returns canned JSON responses in sequence, one per request.
func scriptedServer(t *testing.T, responses []string) *httptest.Server {
	t.Helper()
	i := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if i >= len(responses) {
			t.Errorf("unexpected request #%d", i+1)
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"overflow"}}]}`)
			return
		}
		fmt.Fprint(w, responses[i])
		i++
	}))
}

func collectEvents(events <-chan agentEvent) []agentEvent {
	var out []agentEvent
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func TestRunAgentLoop_BashThenAnswer(t *testing.T) {
	vault := t.TempDir()
	server := scriptedServer(t, []string{
		`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo hi\"}"}}]}}]}`,
		`{"choices":[{"message":{"role":"assistant","content":"Done."}}]}`,
	})
	defer server.Close()

	deps := agentDeps{url: server.URL, apiKey: "k", model: "m", vault: vault, timeout: 5 * time.Second}
	events := make(chan agentEvent)
	msgs := []agentMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "say hi"}}
	go runAgentLoop(context.Background(), deps, msgs, events)
	evs := collectEvents(events)

	var sawToolStart, sawToolResult, sawDone bool
	var finalText string
	for _, ev := range evs {
		switch ev.Kind {
		case evToolStart:
			sawToolStart = true
		case evToolResult:
			sawToolResult = true
		case evAssistantText:
			finalText = ev.Text
		case evDone:
			sawDone = true
		}
	}
	if !sawToolStart || !sawToolResult || !sawDone {
		t.Errorf("missing events: start=%v result=%v done=%v", sawToolStart, sawToolResult, sawDone)
	}
	if finalText != "Done." {
		t.Errorf("final text = %q, want Done.", finalText)
	}
}

func TestRunAgentLoop_StopsAtToolCallCap(t *testing.T) {
	vault := t.TempDir()
	// Always returns a tool call; loop must stop at the cap rather than forever.
	responses := make([]string, maxToolCalls+1)
	for i := range responses {
		responses[i] = `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo x\"}"}}]}}]}`
	}
	server := scriptedServer(t, responses)
	defer server.Close()

	deps := agentDeps{url: server.URL, apiKey: "k", model: "m", vault: vault, timeout: 5 * time.Second}
	events := make(chan agentEvent)
	msgs := []agentMessage{{Role: "user", Content: "loop"}}
	go runAgentLoop(context.Background(), deps, msgs, events)
	evs := collectEvents(events)

	calls := 0
	sawDone := false
	for _, ev := range evs {
		if ev.Kind == evToolStart {
			calls++
		}
		if ev.Kind == evDone {
			sawDone = true
		}
	}
	if calls > maxToolCalls {
		t.Errorf("tool calls = %d, want <= %d", calls, maxToolCalls)
	}
	if !sawDone {
		t.Errorf("loop did not terminate with evDone")
	}
}
