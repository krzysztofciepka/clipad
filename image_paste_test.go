package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func listAssets(t *testing.T, vault string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(vault, "assets", "*.png"))
	if err != nil {
		t.Fatalf("glob assets: %v", err)
	}
	return matches
}

func TestPasteImageInsertsLinkAndSavesAsset(t *testing.T) {
	vault := t.TempDir()
	oldCap := runCapture
	defer func() { runCapture = oldCap }()
	// Probe says image present; read returns bytes. Route by args.
	runCapture = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "TARGETS" || a == "--list-types" {
				return []byte("image/png\n"), nil
			}
		}
		return []byte("PNGBYTES"), nil
	}
	oldEnv := clipEnvForPaste
	defer func() { clipEnvForPaste = oldEnv }()
	clipEnvForPaste = func() clipEnv { return clipX11 }
	oldLook := lookPath
	defer func() { lookPath = oldLook }()
	lookPath = func(string) (string, error) { return "/usr/bin/xclip", nil }

	m := newModel(vault, nil, "", "")
	m.kittyImages = true
	m.currentFile = vault + "/note.md"
	m.editor.SetValue("")
	m.editor.SetImageContext(m.imgReg, true, m.currentNoteDir())

	_ = m.pasteImageOrText()

	if !strings.Contains(m.editor.Value(), "![](assets/img-") {
		t.Errorf("paste did not insert image link: %q", m.editor.Value())
	}
	// Asset file exists.
	if len(listAssets(t, vault)) != 1 {
		t.Errorf("expected one asset file written")
	}
}

// TestPasteFallsBackToTextWhenNoImage verifies that when the clipboard probe
// reports no image/png type, pasteImageOrText takes the plain-text Paste()
// path instead: no asset file is written and no image link is inserted.
//
// The real system clipboard (via atotto/clipboard, used by Paste()'s
// readFromClipboard) is not mocked anywhere in this codebase, so its
// contents are machine-dependent; asserting on the exact pasted text would
// be flaky. Asserting the absence of any image side effect keeps this test
// hermetic while still covering the "no image on clipboard" branch.
func TestPasteFallsBackToTextWhenNoImage(t *testing.T) {
	vault := t.TempDir()
	oldCap := runCapture
	defer func() { runCapture = oldCap }()
	// Probe output has no "image/png" type, so clipboardHasImage is false.
	runCapture = func(name string, args ...string) ([]byte, error) {
		return []byte("UTF8_STRING\n"), nil
	}
	oldEnv := clipEnvForPaste
	defer func() { clipEnvForPaste = oldEnv }()
	clipEnvForPaste = func() clipEnv { return clipX11 }
	oldLook := lookPath
	defer func() { lookPath = oldLook }()
	lookPath = func(string) (string, error) { return "/usr/bin/xclip", nil }

	m := newModel(vault, nil, "", "")
	m.currentFile = vault + "/note.md"
	m.editor.SetValue("")
	m.editor.SetImageContext(m.imgReg, true, m.currentNoteDir())

	_ = m.pasteImageOrText()

	if strings.Contains(m.editor.Value(), "![](assets/") {
		t.Errorf("unexpected image link inserted with no image on clipboard: %q", m.editor.Value())
	}
	if got := listAssets(t, vault); len(got) != 0 {
		t.Errorf("expected no asset file written, got %v", got)
	}
}
