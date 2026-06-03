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

func TestRunAgentLoop_EmitsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer server.Close()

	deps := agentDeps{url: server.URL, apiKey: "k", model: "m", vault: t.TempDir(), timeout: 5 * time.Second}
	events := make(chan agentEvent)
	go runAgentLoop(context.Background(), deps, []agentMessage{{Role: "user", Content: "hi"}}, events)
	evs := collectEvents(events)

	if len(evs) == 0 || evs[len(evs)-1].Kind != evError {
		t.Fatalf("expected last event to be evError, got %+v", evs)
	}
	if evs[len(evs)-1].Err == nil {
		t.Errorf("evError should carry a non-nil Err")
	}
}

// When the cap fires part-way through a multi-tool-call assistant turn, every
// tool_call must still get a paired tool-result message in the persisted
// conversation (API requires 1:1 pairing).
func TestApplyAgentEvent_AppendsTraceAndContent(t *testing.T) {
	turns := []chatTurn{
		{Role: "user", Content: "rename stuff"},
		{Role: "assistant"}, // in-flight placeholder
	}
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolStart, Tool: "bash", Label: "mv a b"})
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolResult, Tool: "bash", Output: "exit 0\n", OK: true})
	turns = applyAgentEvent(turns, agentEvent{Kind: evAssistantText, Text: "Renamed."})

	last := turns[len(turns)-1]
	if len(last.Trace) != 2 {
		t.Fatalf("trace lines = %d, want 2", len(last.Trace))
	}
	if last.Trace[0].Kind != "cmd" || !strings.Contains(last.Trace[0].Text, "mv a b") {
		t.Errorf("trace[0] = %+v", last.Trace[0])
	}
	if last.Trace[1].Text != "✓ (exit 0)" || !last.Trace[1].OK {
		t.Errorf("trace[1] = %+v, want ✓ (exit 0) ok=true", last.Trace[1])
	}
	if last.Content != "Renamed." {
		t.Errorf("content = %q, want Renamed.", last.Content)
	}
}

func TestApplyAgentEvent_FailedBashShowsError(t *testing.T) {
	turns := []chatTurn{{Role: "user", Content: "run"}, {Role: "assistant"}}
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolStart, Tool: "bash", Label: "cat nope"})
	turns = applyAgentEvent(turns, agentEvent{
		Kind: evToolResult, Tool: "bash", OK: false,
		Output: "exit 1\ncat: nope: No such file or directory\n",
	})
	last := turns[len(turns)-1]
	res := last.Trace[len(last.Trace)-1]
	if res.OK {
		t.Errorf("failed result should have OK=false")
	}
	if !strings.Contains(res.Text, "No such file") {
		t.Errorf("trace text %q should contain the error output, not just the exit code", res.Text)
	}
	if strings.Contains(res.Text, "exit 1") {
		t.Errorf("trace text %q should not show the raw exit-code line", res.Text)
	}
}

func TestApplyAgentEvent_SearchAttachesCitations(t *testing.T) {
	turns := []chatTurn{{Role: "user", Content: "q"}, {Role: "assistant"}}
	cites := []citation{{Path: "a.md", StartLine: 1, EndLine: 2}}
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolStart, Tool: "search_vault", Label: "taxes"})
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolResult, Tool: "search_vault", OK: true, Cites: cites})
	last := turns[len(turns)-1]
	if len(last.Citations) != 1 || last.Citations[0].Path != "a.md" {
		t.Errorf("citations = %+v, want a.md", last.Citations)
	}
}

func TestRenderChatScrollback_ShowsTrace(t *testing.T) {
	turns := []chatTurn{
		{Role: "assistant", Content: "ok", Trace: []traceLine{
			{Kind: "cmd", Text: "$ ls"},
			{Kind: "result", Text: "✓ (exit 0)", OK: true},
		}},
	}
	out := renderChatScrollback(turns, 60, false)
	if !strings.Contains(out, "$ ls") {
		t.Errorf("scrollback missing command line: %q", out)
	}
}

func TestRunAgentLoop_CapMidBatchKeepsToolResultsPaired(t *testing.T) {
	vault := t.TempDir()
	responses := make([]string, maxToolCalls+1)
	for i := 0; i < maxToolCalls; i++ {
		responses[i] = `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo x\"}"}}]}}]}`
	}
	// The final turn returns THREE tool calls at once; the cap should already be
	// reached, so all three must be skip-paired.
	responses[maxToolCalls] = `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[` +
		`{"id":"a","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo a\"}"}},` +
		`{"id":"b","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo b\"}"}},` +
		`{"id":"d","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo d\"}"}}` +
		`]}}]}`
	server := scriptedServer(t, responses)
	defer server.Close()

	deps := agentDeps{url: server.URL, apiKey: "k", model: "m", vault: vault, timeout: 5 * time.Second}
	events := make(chan agentEvent)
	msgs := []agentMessage{{Role: "user", Content: "loop"}}
	go runAgentLoop(context.Background(), deps, msgs, events)
	evs := collectEvents(events)

	var final []agentMessage
	for _, ev := range evs {
		if ev.Kind == evDone {
			final = ev.Messages
		}
	}
	if final == nil {
		t.Fatal("no evDone with persisted messages")
	}
	toolCalls, toolResults := 0, 0
	for _, m := range final {
		toolCalls += len(m.ToolCalls)
		if m.Role == "tool" {
			toolResults++
		}
	}
	if toolCalls != toolResults {
		t.Errorf("pairing broken: %d tool_calls vs %d tool results", toolCalls, toolResults)
	}
}
