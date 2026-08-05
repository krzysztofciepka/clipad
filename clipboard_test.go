package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "dest.md")
	os.WriteFile(src, []byte("hello"), 0o644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("dest content = %q, want %q", string(data), "hello")
	}
}

func TestCopyFile_SrcMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nope.md"), filepath.Join(dir, "dst.md"))
	if err == nil {
		t.Error("expected error for missing source, got nil")
	}
}

func TestUniquePath_NoConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	got := uniquePath(path)
	if got != path {
		t.Errorf("uniquePath() = %q, want %q", got, path)
	}
}

func TestUniquePath_Conflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	os.WriteFile(path, []byte(""), 0o644)

	got := uniquePath(path)
	want := filepath.Join(dir, "note (1).md")
	if got != want {
		t.Errorf("uniquePath() = %q, want %q", got, want)
	}
}

func TestUniquePath_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	os.WriteFile(path, []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "note (1).md"), []byte(""), 0o644)

	got := uniquePath(path)
	want := filepath.Join(dir, "note (2).md")
	if got != want {
		t.Errorf("uniquePath() = %q, want %q", got, want)
	}
}

func TestFlashTextPriority(t *testing.T) {
	tests := []struct {
		name          string
		copyFlash     bool
		autoSaveFlash bool
		gitSyncFlash  string
		want          string
	}{
		{"none", false, false, "", ""},
		{"copy only", true, false, "", "Copied"},
		{"autosave only", false, true, "", "Auto-saved"},
		{"gitsync only", false, false, "Synced", "Synced"},
		{"copy beats autosave", true, true, "", "Copied"},
		{"copy beats gitsync", true, false, "Synced", "Copied"},
		{"autosave beats gitsync", false, true, "Backed up", "Auto-saved"},
	}
	for _, tt := range tests {
		got := flashText(tt.copyFlash, tt.autoSaveFlash, tt.gitSyncFlash)
		if got != tt.want {
			t.Errorf("%s: flashText(%v, %v, %q) = %q, want %q",
				tt.name, tt.copyFlash, tt.autoSaveFlash, tt.gitSyncFlash, got, tt.want)
		}
	}
}

func TestFlashCopiedSetsFlagAndReturnsTick(t *testing.T) {
	var m model
	cmd := m.flashCopied()
	if !m.copyFlash {
		t.Error("flashCopied should set copyFlash")
	}
	if cmd == nil {
		t.Fatal("flashCopied should return the fade tick")
	}
	// Not invoking cmd() here: it blocks for the real 2s tick delay. The
	// resulting message type (copyFlashMsg) and the handler's behavior for
	// it are already covered by TestCopyFlashMsgClearsFlag.
}

func TestCopyFlashMsgClearsFlag(t *testing.T) {
	m := model{copyFlash: true}
	next, _ := m.Update(copyFlashMsg{})
	if next.(model).copyFlash {
		t.Error("copyFlashMsg should clear copyFlash")
	}
}

func TestWriteClipboardIsStubbable(t *testing.T) {
	var got string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { got = s; return nil }

	e := newSelectableEditor()
	e.copyToClipboard("payload")

	if got != "payload" {
		t.Errorf("writeClipboard received %q, want %q", got, "payload")
	}
	if e.textClip != "payload" {
		t.Errorf("textClip = %q, want %q", e.textClip, "payload")
	}
}
