package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
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
