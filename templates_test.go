package main

import (
	"testing"
	"time"
)

func TestRenderTemplate_Variables(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	in := "d={{date}} t={{time}} y={{yesterday}} v={{vault}} c={{date:02 Jan 2006}}"
	want := "d=2026-05-25 t=14:30 y=2026-05-24 v=/tmp/vault c=25 May 2026"
	got := renderTemplate(in, now, "/tmp/vault")
	if got != want {
		t.Errorf("renderTemplate:\n got  %q\n want %q", got, want)
	}
}

func TestRenderTemplate_UnknownPlaceholdersUntouched(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	in := "{{foo}} {{date}} {{bar:x}} literal {{ }}"
	want := "{{foo}} 2026-05-25 {{bar:x}} literal {{ }}"
	got := renderTemplate(in, now, "/v")
	if got != want {
		t.Errorf("renderTemplate:\n got  %q\n want %q", got, want)
	}
}
