package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveInboxPath_EmptyDefaultsToInboxMd(t *testing.T) {
	got := resolveInboxPath("/vault", "")
	want := filepath.Join("/vault", "inbox.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_BareFilenameVaultRelative(t *testing.T) {
	got := resolveInboxPath("/vault", "scratch.md")
	want := filepath.Join("/vault", "scratch.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_SubpathVaultRelative(t *testing.T) {
	got := resolveInboxPath("/vault", "journals/inbox.md")
	want := filepath.Join("/vault", "journals", "inbox.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_AbsoluteUsedAsIs(t *testing.T) {
	got := resolveInboxPath("/vault", "/tmp/inbox.md")
	want := "/tmp/inbox.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_TildeExpanded(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	got := resolveInboxPath("/vault", "~/scratch.md")
	want := filepath.Join(tmp, "scratch.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_DotDotCleaned(t *testing.T) {
	got := resolveInboxPath("/vault", "a/../b.md")
	want := filepath.Join("/vault", "b.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatCaptureLine_FixedTime(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "hello world")
	want := "- 2026-04-27 14:22 — hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatCaptureLine_EmDashLiteralBytes(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "x")
	if !strings.Contains(got, "—") {
		t.Errorf("output %q missing em-dash U+2014", got)
	}
	if !strings.Contains(got, " — ") {
		t.Errorf("output %q does not contain ' — ' as separator", got)
	}
}

func TestFormatCaptureLine_MultilineEmbedded(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "first\nsecond\nthird")
	want := "- 2026-04-27 14:22 — first\nsecond\nthird"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatCaptureLine_EmptyTextSafe(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "")
	want := "- 2026-04-27 14:22 — "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendToInboxFile_CreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")

	if err := appendToInboxFile(path, "- 2026-04-27 14:22 — hi"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "- 2026-04-27 14:22 — hi\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_CreatesMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journals", "inbox.md")

	if err := appendToInboxFile(path, "- t — x"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestAppendToInboxFile_PreservesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")
	os.WriteFile(path, []byte("existing line\n"), 0o644)

	if err := appendToInboxFile(path, "- t — new"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	want := "existing line\n- t — new\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_AddsNewlineWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")
	os.WriteFile(path, []byte("existing line"), 0o644)

	if err := appendToInboxFile(path, "- t — new"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	want := "existing line\n- t — new\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_PreservesBlankLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")
	os.WriteFile(path, []byte("a\n\n"), 0o644)

	if err := appendToInboxFile(path, "- t — new"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	want := "a\n\n- t — new\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_TwoCapturesInSequence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")

	if err := appendToInboxFile(path, "- t — one"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := appendToInboxFile(path, "- t — two"); err != nil {
		t.Fatalf("second: %v", err)
	}

	data, _ := os.ReadFile(path)
	want := "- t — one\n- t — two\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_FileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")

	if err := appendToInboxFile(path, "- t — x"); err != nil {
		t.Fatalf("append: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestEnsureTrailingNewline_Empty(t *testing.T) {
	if got := ensureTrailingNewline(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestEnsureTrailingNewline_AlreadyHasOne(t *testing.T) {
	if got := ensureTrailingNewline("foo\n"); got != "foo\n" {
		t.Errorf("got %q, want %q", got, "foo\n")
	}
}

func TestEnsureTrailingNewline_AddsOne(t *testing.T) {
	if got := ensureTrailingNewline("foo"); got != "foo\n" {
		t.Errorf("got %q, want %q", got, "foo\n")
	}
}

func TestEnsureTrailingNewline_OnlyNewline(t *testing.T) {
	if got := ensureTrailingNewline("\n"); got != "\n" {
		t.Errorf("got %q, want %q", got, "\n")
	}
}

func TestWriteNewFile_CreatesIfMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")

	if err := writeNewFile(path, "hello\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello\n" {
		t.Errorf("got %q, want %q", string(data), "hello\n")
	}
}

func TestWriteNewFile_RefusesIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.md")
	os.WriteFile(path, []byte("preexisting"), 0o644)

	err := writeNewFile(path, "should not overwrite")
	if !os.IsExist(err) {
		t.Fatalf("err = %v, want os.IsExist", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "preexisting" {
		t.Errorf("file overwritten: %q", string(data))
	}
}

func TestWriteNewFile_ParentMissingReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope", "child.md")

	err := writeNewFile(path, "x")
	if err == nil {
		t.Error("expected error when parent dir is missing")
	}
	if os.IsExist(err) {
		t.Errorf("err = %v, want a 'no such file' error, not ErrExist", err)
	}
}

func TestCaptureView_ContainsInputAndPath(t *testing.T) {
	out := captureView("typed text", "/vault/inbox.md", 80, 24)
	if out == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(out, "typed text") {
		t.Errorf("render missing input text: %q", out)
	}
	if !strings.Contains(out, "/vault/inbox.md") {
		t.Errorf("render missing inbox path: %q", out)
	}
}

func TestHandleCapture_EscClosesModal(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	m.captureInput.SetValue("not committed")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestHandleCapture_EmptyEnterCancelsSilently(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestHandleCapture_WhitespaceOnlyEnterCancelsSilently(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	m.captureInput.SetValue("   \n\t  ")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}
