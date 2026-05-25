package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

func TestSeedDefaultTemplate_CreatesWhenAbsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := seedDefaultTemplate(); err != nil {
		t.Fatalf("seedDefaultTemplate: %v", err)
	}
	path := filepath.Join(templatesDir(), "daily.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("daily.md not created: %v", err)
	}
	if !strings.Contains(string(data), "{{date") {
		t.Errorf("seeded template missing date variable:\n%s", data)
	}
}

func TestSeedDefaultTemplate_DoesNotOverwrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(templatesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(templatesDir(), "daily.md")
	if err := os.WriteFile(path, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := seedDefaultTemplate(); err != nil {
		t.Fatalf("seedDefaultTemplate: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading template after seed: %v", err)
	}
	if string(data) != "custom" {
		t.Errorf("seed overwrote existing template: got %q", data)
	}
}

func TestListTemplates_SortsAndFiltersMd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := templatesDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zzz.md", "daily.md", "note.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := listTemplates()
	if err != nil {
		t.Fatalf("listTemplates: %v", err)
	}
	want := []string{"daily.md", "zzz.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("listTemplates = %v, want %v", got, want)
	}
}

func TestListTemplates_MissingDirReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := listTemplates()
	if err != nil {
		t.Fatalf("listTemplates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listTemplates on missing dir = %v, want empty", got)
	}
}

func TestRenderTemplate_EmptyLayoutFallsBackToDefault(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	if got := renderTemplate("{{date:}}", now, "/v"); got != "2026-05-25" {
		t.Errorf("renderTemplate({{date:}}) = %q, want 2026-05-25", got)
	}
}

func TestOpenDailyNote_CreatesFromTemplate(t *testing.T) {
	m := newTestModel(t)
	m.openDailyNote()
	if m.errMsg != "" {
		t.Fatalf("unexpected errMsg: %s", m.errMsg)
	}
	today := time.Now().Format("2006-01-02")
	want := filepath.Join(m.vault, "daily", today+".md")
	if m.currentFile != want {
		t.Errorf("currentFile = %q, want %q", m.currentFile, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("daily note not created: %v", err)
	}
	body := m.editor.Value()
	if strings.Contains(body, "{{date") || strings.Contains(body, "{{yesterday") {
		t.Errorf("template not rendered:\n%s", body)
	}
	if !strings.Contains(body, today[:4]) { // year present somewhere
		t.Errorf("rendered body missing date:\n%s", body)
	}
}

func TestOpenDailyNote_OpensExistingWithoutRerender(t *testing.T) {
	m := newTestModel(t)
	today := time.Now().Format("2006-01-02")
	dir := filepath.Join(m.vault, "daily")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, today+".md")
	if err := os.WriteFile(path, []byte("EXISTING {{date}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.openDailyNote()
	if m.editor.Value() != "EXISTING {{date}}" {
		t.Errorf("existing note was modified: %q", m.editor.Value())
	}
}

func TestUpdate_AltD_OpensDailyNote(t *testing.T) {
	m := newTestModel(t)
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true}
	next, _ := m.Update(key)
	nm := next.(model)
	today := time.Now().Format("2006-01-02")
	want := filepath.Join(nm.vault, "daily", today+".md")
	if nm.currentFile != want {
		t.Errorf("after Alt+D currentFile = %q, want %q", nm.currentFile, want)
	}
}

func seedTemplate(t *testing.T, name, body string) {
	t.Helper()
	dir := templatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTemplatePick_EnterAdvancesToName(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputTemplatePick
	m.templateList = []string{"daily.md", "meeting.md"}
	m.templateCursor = 1
	next, _ := m.handleTemplatePick(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputTemplateName {
		t.Errorf("inputMode = %v, want inputTemplateName", nm.inputMode)
	}
	if nm.templateChosen != "meeting.md" {
		t.Errorf("templateChosen = %q, want meeting.md", nm.templateChosen)
	}
}

func TestTemplatePick_EscCancels(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputTemplatePick
	m.templateList = []string{"daily.md"}
	next, _ := m.handleTemplatePick(pressEsc())
	if next.(model).inputMode != inputNone {
		t.Errorf("Esc did not cancel picker")
	}
}

func TestTemplateName_CreatesRenderedFileAndOpens(t *testing.T) {
	m := newTestModel(t)
	seedTemplate(t, "note.md", "Hello {{date}}")
	m.inputMode = inputTemplateName
	m.templateChosen = "note.md"
	m.templateTargetDir = m.vault
	m.templateNameInput.SetValue("myidea")
	next, _ := m.handleTemplateName(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	path := filepath.Join(m.vault, "myidea.md")
	if nm.currentFile != path {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if string(data) != "Hello "+today {
		t.Errorf("rendered content = %q, want %q", data, "Hello "+today)
	}
}

func TestTemplateName_RejectsExistingFile(t *testing.T) {
	m := newTestModel(t)
	seedTemplate(t, "note.md", "x")
	existing := filepath.Join(m.vault, "dupe.md")
	if err := os.WriteFile(existing, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.inputMode = inputTemplateName
	m.templateChosen = "note.md"
	m.templateTargetDir = m.vault
	m.templateNameInput.SetValue("dupe.md")
	next, _ := m.handleTemplateName(pressEnter())
	nm := next.(model)
	if nm.errMsg == "" {
		t.Errorf("expected errMsg for existing file")
	}
	if nm.inputMode != inputTemplateName {
		t.Errorf("inputMode = %v after rejection, want inputTemplateName (user must be able to retry)", nm.inputMode)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "ORIGINAL" {
		t.Errorf("existing file overwritten: %q", data)
	}
}

func TestUpdate_AltT_OpensPicker(t *testing.T) {
	m := newTestModel(t)
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true}
	next, _ := m.Update(key)
	nm := next.(model)
	if nm.inputMode != inputTemplatePick {
		t.Errorf("after Alt+T inputMode = %v, want inputTemplatePick", nm.inputMode)
	}
	if len(nm.templateList) == 0 {
		t.Errorf("templateList empty; expected seeded daily.md")
	}
}

func TestAltD_DirtyDetoursToUnsavedGuard(t *testing.T) {
	m := newTestModel(t)
	m.editor.SetValue("unsaved edits") // cleanContent is "" → dirty
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true}
	next, _ := m.Update(key)
	nm := next.(model)
	if nm.inputMode != inputUnsavedGuard {
		t.Errorf("inputMode = %v, want inputUnsavedGuard", nm.inputMode)
	}
	if nm.pendingAction != pendingDailyNote {
		t.Errorf("pendingAction = %v, want pendingDailyNote", nm.pendingAction)
	}
	today := time.Now().Format("2006-01-02")
	if _, err := os.Stat(filepath.Join(nm.vault, "daily", today+".md")); err == nil {
		t.Errorf("daily note was created despite unsaved guard")
	}
}

func TestAltT_DirtyDetoursToUnsavedGuard(t *testing.T) {
	m := newTestModel(t)
	m.editor.SetValue("unsaved edits")
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true}
	next, _ := m.Update(key)
	nm := next.(model)
	if nm.inputMode != inputUnsavedGuard {
		t.Errorf("inputMode = %v, want inputUnsavedGuard", nm.inputMode)
	}
	if nm.pendingAction != pendingTemplatePicker {
		t.Errorf("pendingAction = %v, want pendingTemplatePicker", nm.pendingAction)
	}
}

func TestExecutePending_DailyNoteResumes(t *testing.T) {
	m := newTestModel(t)
	m.pendingAction = pendingDailyNote
	next, _ := m.executePendingAction()
	nm := next.(model)
	today := time.Now().Format("2006-01-02")
	want := filepath.Join(nm.vault, "daily", today+".md")
	if nm.currentFile != want {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, want)
	}
	if nm.pendingAction != pendingNone {
		t.Errorf("pendingAction = %v, want pendingNone", nm.pendingAction)
	}
}

func TestExecutePending_TemplatePickerResumes(t *testing.T) {
	m := newTestModel(t)
	m.pendingAction = pendingTemplatePicker
	next, _ := m.executePendingAction()
	nm := next.(model)
	if nm.inputMode != inputTemplatePick {
		t.Errorf("inputMode = %v, want inputTemplatePick", nm.inputMode)
	}
	if nm.pendingAction != pendingNone {
		t.Errorf("pendingAction = %v, want pendingNone", nm.pendingAction)
	}
}
