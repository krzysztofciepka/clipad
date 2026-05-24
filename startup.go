package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type startupKind int

const (
	startupNone startupKind = iota
	startupNewNote      // --new: new note in the vault root
	startupNewNoteInDir // path resolves to a directory
	startupOpenFile     // path resolves to a file
)

// startupAction describes the one-shot action to perform when clipad launches
// with command-line arguments. It is resolved before the TUI starts and applied
// once on the first WindowSizeMsg.
type startupAction struct {
	kind        startupKind
	path        string // resolved absolute path (file for open; directory for new-note kinds)
	preview     bool   // open the file in preview mode (only meaningful for startupOpenFile)
	hideTree    bool   // hide the file tree on launch
	needsCreate bool   // create an empty file (plus parents) before opening
	needsMkdir  bool   // create the directory before starting the new note
}

// resolveStartup classifies command-line arguments into a startupAction. It
// performs read-only stat checks but no filesystem writes, so it is safe to
// unit test. cwd and vault are passed in (not read from globals) so tests need
// no chdir and can run in parallel.
func resolveStartup(preview, newNote bool, pathArg, cwd, vault string) (startupAction, error) {
	if newNote {
		// --new deliberately keeps the file tree visible (hideTree stays false).
		return startupAction{kind: startupNewNote, path: vault, hideTree: false}, nil
	}
	if pathArg == "" {
		if preview {
			return startupAction{}, fmt.Errorf("-p requires a file path")
		}
		return startupAction{kind: startupNone}, nil
	}

	trailingSlash := strings.HasSuffix(pathArg, "/")

	p := pathArg
	// Expand a leading "~/" (or a bare "~") to the home directory. The "~user"
	// form is not supported — such a path is treated literally.
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return startupAction{}, fmt.Errorf("cannot expand ~: %w", err)
		}
		p = home + p[1:]
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	abs := filepath.Clean(p)

	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir():
		return startupAction{kind: startupNewNoteInDir, path: abs, hideTree: true}, nil
	case err == nil:
		return startupAction{kind: startupOpenFile, path: abs, preview: preview, hideTree: true}, nil
	case os.IsNotExist(err):
		if trailingSlash {
			return startupAction{kind: startupNewNoteInDir, path: abs, hideTree: true, needsMkdir: true}, nil
		}
		return startupAction{kind: startupOpenFile, path: abs, preview: preview, hideTree: true, needsCreate: true}, nil
	default:
		return startupAction{}, fmt.Errorf("cannot access %s: %w", abs, err)
	}
}

// applyStartup mutates the model to reflect the resolved startup action. It runs
// once, on the first WindowSizeMsg, after recalcLayout has sized the panes. All
// referenced paths already exist (prepareStartup ran in main). It returns a
// command to focus the editor where appropriate.
func (m *model) applyStartup() tea.Cmd {
	m.treeHidden = m.startup.hideTree
	m.recalcLayout() // re-flow now that treeHidden may have changed

	switch m.startup.kind {
	case startupOpenFile:
		m.openFile(m.startup.path)
		if m.startup.preview {
			vp := viewport.New(m.editorWidth-2, m.editorHeight)
			vp.SetContent(wordWrap(m.editor.Value(), m.editorWidth-4))
			m.preview = vp
			m.editorMode = modePreview
			m.editor.Blur()
			// Unlike previewSelectedFile (which keeps the tree focused for
			// browsing), the editor panel must be active so keystrokes reach
			// handleEditorKeys, where a rune in modePreview switches to edit.
			m.activePanel = editorPanel
			return nil
		}
		m.editorMode = modeEdit
		m.activePanel = editorPanel
		return m.editor.Focus()

	case startupNewNote, startupNewNoteInDir:
		m.newNoteDir = m.startup.path
		m.currentFile = ""
		m.editor.ClearHistory()
		m.editor.SetValue("")
		m.cleanContent = ""
		m.errMsg = ""
		m.editorMode = modeEdit
		m.activePanel = editorPanel
		return m.editor.Focus()
	}
	return nil
}

// prepareStartup performs the filesystem side effects implied by an action:
// creating a missing file (and its parents) or a missing directory. It is
// called from main() before the TUI starts so errors can exit cleanly.
func prepareStartup(a startupAction) error {
	switch {
	case a.needsCreate:
		if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		return f.Close()
	case a.needsMkdir:
		return os.MkdirAll(a.path, 0o755)
	}
	return nil
}
