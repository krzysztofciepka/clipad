package main

import (
	"testing"
)

func TestCopyLoneImageWritesBytes(t *testing.T) {
	vault := t.TempDir()
	// Create an asset the link points to.
	abs, _ := saveAsset(vault, []byte("REALPNG"), "2026-07-03")
	rel := assetRelPath(vault, abs)

	var wrote []byte
	oldW := runWithStdin
	defer func() { runWithStdin = oldW }()
	runWithStdin = func(stdin []byte, name string, args ...string) error { wrote = stdin; return nil }

	oldLook := lookPath
	defer func() { lookPath = oldLook }()
	lookPath = func(string) (string, error) { return "/usr/bin/wl-copy", nil }

	oldEnv := clipEnvForPaste
	defer func() { clipEnvForPaste = oldEnv }()
	clipEnvForPaste = func() clipEnv { return clipWayland }

	m := newModel(vault, nil, "", "")
	m.currentFile = vault + "/note.md"
	m.editor.SetValue("![](" + rel + ")")
	m.editor.moveTo(0, 0)

	target, ok := m.editor.LoneImageElement()
	if !ok {
		t.Fatal("cursor on image line should report lone element")
	}
	if !m.copyImageElement(target, false) {
		t.Fatal("copyImageElement should succeed")
	}
	if string(wrote) != "REALPNG" {
		t.Errorf("wrote %q to clipboard, want REALPNG", wrote)
	}
}
