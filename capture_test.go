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

func TestCapture_OpenCleanInbox_WritesAndReloads(t *testing.T) {
	m := newTestModel(t)
	inboxPath := filepath.Join(m.vault, "inbox.md")
	if err := os.WriteFile(inboxPath, []byte("- old — first\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m.currentFile = inboxPath
	m.editor.SetValue("- old — first\n")
	m.cleanContent = "- old — first\n"
	if m.isDirty() {
		t.Fatal("editor should be clean after seed")
	}

	m.inputMode = inputCapture
	m.captureInput.SetValue("new entry")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if cmd == nil {
		t.Fatal("expected cmd")
	}

	msg := cmd().(captureAppendedMsg)
	if msg.err != nil {
		t.Fatalf("append err: %v", msg.err)
	}
	if !msg.reloadOpen {
		t.Error("reloadOpen = false, want true (inbox is open and was clean)")
	}

	data, _ := os.ReadFile(inboxPath)
	if !strings.Contains(string(data), "- old — first\n") {
		t.Errorf("disk missing original line: %q", string(data))
	}
	if !strings.Contains(string(data), " — new entry\n") {
		t.Errorf("disk missing new line: %q", string(data))
	}

	next2, _ := nm.Update(msg)
	nm2 := next2.(model)
	if nm2.editor.Value() != string(data) {
		t.Errorf("editor not reloaded: got %q, want %q", nm2.editor.Value(), string(data))
	}
	if nm2.cleanContent != string(data) {
		t.Error("cleanContent not updated; editor would be dirty")
	}
}

func TestCapture_OpenDirtyInbox_AppendsInMemory(t *testing.T) {
	m := newTestModel(t)
	inboxPath := filepath.Join(m.vault, "inbox.md")
	original := "- old — first\n"
	if err := os.WriteFile(inboxPath, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m.currentFile = inboxPath
	m.editor.SetValue("- old — first\nDIRTY EDIT\n")
	m.cleanContent = original
	if !m.isDirty() {
		t.Fatal("editor should be dirty after the edit")
	}

	m.inputMode = inputCapture
	m.captureInput.SetValue("from capture")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if cmd != nil {
		t.Errorf("expected nil cmd (no disk write), got %v", cmd)
	}

	data, _ := os.ReadFile(inboxPath)
	if string(data) != original {
		t.Errorf("disk modified; got %q, want %q", string(data), original)
	}

	got := nm.editor.Value()
	if !strings.Contains(got, "DIRTY EDIT") {
		t.Errorf("editor lost dirty edit: %q", got)
	}
	if !strings.Contains(got, " — from capture") {
		t.Errorf("editor missing captured line: %q", got)
	}

	if !nm.isDirty() {
		t.Error("editor should still be dirty after in-memory capture")
	}
}

func TestHandleDelegate_EscClosesModal(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("foo")

	next, cmd := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestHandleDelegate_EmptyEnterIgnored(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputDelegateName

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want inputDelegateName (modal stays open)", nm.inputMode)
	}
}

// delegateSetup creates a temp vault, writes a source file, opens it
// in the model with a selection covering cols [selectFromCol, selectToCol)
// on row 0. Returns the configured model and the source path.
func delegateSetup(t *testing.T, srcContent string, selectFromCol, selectToCol int) (model, string) {
	t.Helper()
	m := newTestModel(t)
	srcPath := filepath.Join(m.vault, "src.md")
	if err := os.WriteFile(srcPath, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	m.currentFile = srcPath
	m.editor.SetValue(srcContent)
	m.cleanContent = srcContent
	m.editor.selActive = true
	m.editor.selAnchorLine = 0
	m.editor.selAnchorCol = selectFromCol
	m.editor.MoveTo(0, selectToCol)
	m.activePanel = editorPanel
	return m, srcPath
}

func TestDelegate_HappyPath(t *testing.T) {
	m, srcPath := delegateSetup(t, "foo bar baz", 4, 7)
	srcDir := filepath.Dir(srcPath)

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("notes")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}

	dstPath := filepath.Join(srcDir, "notes.md")
	dst, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if string(dst) != "bar\n" {
		t.Errorf("destination = %q, want %q", string(dst), "bar\n")
	}

	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("source missing: %v", err)
	}
	if string(src) != "foo  baz" {
		t.Errorf("source on disk = %q, want %q", string(src), "foo  baz")
	}

	if !strings.HasPrefix(nm.editor.Value(), "foo  baz") {
		t.Errorf("editor = %q, want post-cut content", nm.editor.Value())
	}
	if nm.isDirty() {
		t.Error("editor should be clean after delegate save")
	}
}

func TestDelegate_AutoAppendsMd(t *testing.T) {
	m, srcPath := delegateSetup(t, "abcdef", 0, 3)
	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("typed")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	_ = next

	dstPath := filepath.Join(filepath.Dir(srcPath), "typed.md")
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf(".md not auto-appended; expected %s", dstPath)
	}
}

func TestDelegate_DirtySource_SinglePersistedSave(t *testing.T) {
	m := newTestModel(t)
	srcPath := filepath.Join(m.vault, "src.md")
	onDisk := "foo bar baz\n"
	if err := os.WriteFile(srcPath, []byte(onDisk), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m.currentFile = srcPath
	m.editor.SetValue("foo bar baz\nDIRTY\n")
	m.cleanContent = onDisk
	if !m.isDirty() {
		t.Fatal("editor must be dirty for this scenario")
	}
	m.editor.selActive = true
	m.editor.selAnchorLine = 0
	m.editor.selAnchorCol = 4
	m.editor.MoveTo(0, 7)
	m.activePanel = editorPanel

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("notes")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.isDirty() {
		t.Error("editor should be clean after delegate save")
	}

	src, _ := os.ReadFile(srcPath)
	if !strings.Contains(string(src), "foo  baz") {
		t.Errorf("on-disk source missing cut: %q", string(src))
	}
	if !strings.Contains(string(src), "DIRTY") {
		t.Errorf("on-disk source missing dirty edit: %q", string(src))
	}

	dst, err := os.ReadFile(filepath.Join(m.vault, "notes.md"))
	if err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if string(dst) != "bar\n" {
		t.Errorf("destination = %q, want %q", string(dst), "bar\n")
	}
}

func TestDelegate_CollisionRefused(t *testing.T) {
	m, srcPath := delegateSetup(t, "abcdef", 0, 3)
	srcDir := filepath.Dir(srcPath)
	collidePath := filepath.Join(srcDir, "taken.md")
	if err := os.WriteFile(collidePath, []byte("preexisting"), 0o644); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("taken")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want stays inputDelegateName", nm.inputMode)
	}
	if nm.errMsg == "" {
		t.Error("errMsg should be set on collision")
	}

	data, _ := os.ReadFile(collidePath)
	if string(data) != "preexisting" {
		t.Errorf("collision file overwritten: %q", string(data))
	}
	src, _ := os.ReadFile(srcPath)
	if string(src) != "abcdef" {
		t.Errorf("source modified despite collision refusal: %q", string(src))
	}
	if nm.editor.Value() != "abcdef" {
		t.Errorf("editor modified despite collision refusal: %q", nm.editor.Value())
	}
}

func TestHandleDelegate_SlashRejected(t *testing.T) {
	m := newTestModel(t)
	srcDir := m.vault
	srcPath := filepath.Join(srcDir, "src.md")
	os.WriteFile(srcPath, []byte("hello world"), 0o644)
	m.currentFile = srcPath
	m.editor.SetValue("hello world")
	m.cleanContent = "hello world"
	m.editor.selActive = true
	m.editor.selAnchorLine = 0
	m.editor.selAnchorCol = 0
	m.editor.MoveTo(0, 5)

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("subdir/foo")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want stays inputDelegateName", nm.inputMode)
	}
	if nm.errMsg == "" {
		t.Error("errMsg should be set on slash rejection")
	}
	if _, err := os.Stat(filepath.Join(srcDir, "subdir")); err == nil {
		t.Error("subdir was created — handler should have rejected before any IO")
	}
}

func TestCapture_OpenCleanInbox_PreservesCursor(t *testing.T) {
	m := newTestModel(t)
	inboxPath := filepath.Join(m.vault, "inbox.md")
	original := "line zero\nline one\nline two\n"
	os.WriteFile(inboxPath, []byte(original), 0o644)

	m.currentFile = inboxPath
	m.editor.SetValue(original)
	m.cleanContent = original
	m.editor.MoveTo(2, 3)

	m.inputMode = inputCapture
	m.captureInput.SetValue("appended")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	msg := cmd().(captureAppendedMsg)
	next2, _ := nm.Update(msg)
	nm2 := next2.(model)

	gotLine, gotCol := editorCursorPos(nm2.editor)
	if gotLine != 2 || gotCol != 3 {
		t.Errorf("cursor = (%d, %d), want (2, 3)", gotLine, gotCol)
	}
}

func TestCapture_ClosedInbox_WritesToDisk(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	m.captureInput.SetValue("hello world")
	// m.currentFile is "" — inbox is not open

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for non-empty capture")
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}

	resultMsg := cmd().(captureAppendedMsg)
	if resultMsg.err != nil {
		t.Fatalf("captureAppendedMsg err: %v", resultMsg.err)
	}
	if resultMsg.reloadOpen {
		t.Errorf("reloadOpen = true, want false (inbox not open)")
	}

	inboxPath := filepath.Join(m.vault, "inbox.md")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, " — hello world\n") {
		t.Errorf("inbox content %q does not contain ' — hello world\\n'", got)
	}
	if !strings.HasPrefix(got, "- ") {
		t.Errorf("inbox content %q does not start with bullet", got)
	}
}
