package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writePattern creates dir/name/system.md with the given body.
func writePattern(t *testing.T, dir, name, system string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "system.md"), []byte(system), 0o644); err != nil {
		t.Fatalf("write system.md for %s: %v", name, err)
	}
}

func TestListFabricPatterns_SortedAndFiltered(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SUMMARIZE")
	writePattern(t, dir, "analyze_claims", "# ANALYZE")
	// Empty system.md: not a usable pattern.
	writePattern(t, dir, "hollow", "")
	// Directory without a system.md at all.
	if err := os.MkdirAll(filepath.Join(dir, "notapattern"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Loose files alongside the pattern directories.
	if err := os.WriteFile(filepath.Join(dir, "loaded"), nil, 0o644); err != nil {
		t.Fatalf("write loaded: %v", err)
	}

	got := listFabricPatterns(dir)
	if len(got) != 2 {
		t.Fatalf("got %d patterns (%v), want 2", len(got), got)
	}
	if got[0].Name != "analyze_claims" || got[1].Name != "summarize" {
		t.Errorf("names = [%s, %s], want [analyze_claims, summarize]", got[0].Name, got[1].Name)
	}
}

func TestListFabricPatterns_MissingDirIsNotAnError(t *testing.T) {
	if got := listFabricPatterns(filepath.Join(t.TempDir(), "absent")); got != nil {
		t.Errorf("got %v, want nil when fabric is not installed", got)
	}
}

func TestListFabricPatterns_AttachesExplanations(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SUMMARIZE")
	writePattern(t, dir, "undocumented", "# X")
	explanations := "# Brief one-line summary\n\n" +
		"1. **summarize**: Summarize content into sections.\n" +
		"2. **absent_pattern**: Not installed here.\n"
	if err := os.WriteFile(filepath.Join(dir, "pattern_explanations.md"), []byte(explanations), 0o644); err != nil {
		t.Fatalf("write explanations: %v", err)
	}

	got := listFabricPatterns(dir)
	if len(got) != 2 {
		t.Fatalf("got %d patterns, want 2", len(got))
	}
	if got[0].Description != "Summarize content into sections." {
		t.Errorf("summarize description = %q", got[0].Description)
	}
	if got[1].Description != "" {
		t.Errorf("undocumented description = %q, want empty", got[1].Description)
	}
}

func TestParsePatternExplanations_IgnoresNonEntryLines(t *testing.T) {
	in := []byte("# Heading\n\n- Key pattern to use: **suggest_pattern**, suggests patterns.\n" +
		"12. **analyze_claims**: Analyse and rate truth claims.\n")
	got := parsePatternExplanations(in)
	if len(got) != 1 {
		t.Fatalf("got %d entries (%v), want 1", len(got), got)
	}
	if got["analyze_claims"] != "Analyse and rate truth claims." {
		t.Errorf("analyze_claims = %q", got["analyze_claims"])
	}
}

func TestFabricPatternsDir_HonoursEnvOverride(t *testing.T) {
	t.Setenv("FABRIC_CONFIG_HOME", "/custom/fabric")
	if got := fabricPatternsDir(); got != "/custom/fabric/patterns" {
		t.Errorf("fabricPatternsDir() = %q, want /custom/fabric/patterns", got)
	}
}

func TestLoadFabricPattern_ReturnsSystemVerbatim(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# IDENTITY\n\nYou summarize.\n")

	system, user, err := loadFabricPattern(dir, "summarize")
	if err != nil {
		t.Fatalf("loadFabricPattern: %v", err)
	}
	if system != "# IDENTITY\n\nYou summarize.\n" {
		t.Errorf("system = %q, want the file verbatim", system)
	}
	if user != "" {
		t.Errorf("user = %q, want empty when there is no user.md", user)
	}
}

func TestLoadFabricPattern_BlankUserMdIsIgnored(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SYS")
	if err := os.WriteFile(filepath.Join(dir, "summarize", "user.md"), []byte("\n  \n"), 0o644); err != nil {
		t.Fatalf("write user.md: %v", err)
	}

	_, user, err := loadFabricPattern(dir, "summarize")
	if err != nil {
		t.Fatalf("loadFabricPattern: %v", err)
	}
	if user != "" {
		t.Errorf("user = %q, want empty for a whitespace-only user.md", user)
	}
}

func TestLoadFabricPattern_KeepsPopulatedUserMd(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SYS")
	if err := os.WriteFile(filepath.Join(dir, "summarize", "user.md"), []byte("EXTRA CONTEXT"), 0o644); err != nil {
		t.Fatalf("write user.md: %v", err)
	}

	_, user, err := loadFabricPattern(dir, "summarize")
	if err != nil {
		t.Fatalf("loadFabricPattern: %v", err)
	}
	if user != "EXTRA CONTEXT" {
		t.Errorf("user = %q, want EXTRA CONTEXT", user)
	}
}

func TestLoadFabricPattern_MissingPatternErrors(t *testing.T) {
	if _, _, err := loadFabricPattern(t.TempDir(), "nope"); err == nil {
		t.Error("loadFabricPattern on a missing pattern returned nil error")
	}
}

func TestRunFabricPatternStream_SendsSystemMdAsSystemMessage(t *testing.T) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Messages []message `json:"messages"`
	}
	captured := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		captured <- req
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	// shortcutProviderURL has no test seam, so point the pattern run at the
	// test server through the fabricStreamURL indirection.
	origURL := fabricStreamURL
	fabricStreamURL = func(string) string { return server.URL }
	defer func() { fabricStreamURL = origURL }()

	chunks, errs := runFabricPatternStream(context.Background(),
		"# IDENTITY\n\nYou summarize.", "", "note body",
		"openrouter", map[string]string{"api_key": "k", "model": "m"})
	for range chunks {
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	default:
	}

	req := <-captured
	if len(req.Messages) < 2 {
		t.Fatalf("got %d messages, want 2", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "# IDENTITY\n\nYou summarize." {
		t.Errorf("system message = %+v, want system.md verbatim", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "note body" {
		t.Errorf("user message = %+v, want the note body alone", req.Messages[1])
	}
}

func TestRunFabricPatternStream_PrependsUserMd(t *testing.T) {
	if got := fabricUserMessage("PREAMBLE", "note body"); got != "PREAMBLE\n\nnote body" {
		t.Errorf("fabricUserMessage = %q, want PREAMBLE\\n\\nnote body", got)
	}
	if got := fabricUserMessage("", "note body"); got != "note body" {
		t.Errorf("fabricUserMessage with no user.md = %q, want note body", got)
	}
}
