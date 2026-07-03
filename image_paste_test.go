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
