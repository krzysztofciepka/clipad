package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseImageElement(t *testing.T) {
	cases := []struct {
		line   string
		target string
		ok     bool
	}{
		{"![](assets/img-2026-07-03-abc12345.png)", "assets/img-2026-07-03-abc12345.png", true},
		{"![alt text](assets/pic.jpeg)", "assets/pic.jpeg", true},
		{"  ![](assets/pic.png)  ", "assets/pic.png", true},
		{"![](assets/notes.md)", "", false},
		{"text before ![](assets/pic.png)", "", false},
		{"![](assets/pic.png) trailing text", "", false},
		{"not an image", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		target, ok := parseImageElement(c.line)
		if ok != c.ok || target != c.target {
			t.Errorf("parseImageElement(%q) = (%q, %v), want (%q, %v)", c.line, target, ok, c.target, c.ok)
		}
	}
}

func TestAssetFilename(t *testing.T) {
	// sha256("hello") starts with 2cf24dba...
	got := assetFilename([]byte("hello"), "2026-07-03")
	want := "img-2026-07-03-2cf24dba.png"
	if got != want {
		t.Errorf("assetFilename = %q, want %q", got, want)
	}
}

func TestSaveAssetIsIdempotent(t *testing.T) {
	vault := t.TempDir()
	data := []byte("PNGDATA")
	p1, err := saveAsset(vault, data, "2026-07-03")
	if err != nil {
		t.Fatal(err)
	}
	info1, _ := os.Stat(p1)
	p2, err := saveAsset(vault, data, "2026-07-03")
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Errorf("saveAsset not stable: %q vs %q", p1, p2)
	}
	info2, _ := os.Stat(p2)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("second save rewrote the file; want no-op")
	}
	if filepath.Dir(p1) != filepath.Join(vault, "assets") {
		t.Errorf("asset dir = %q, want %q", filepath.Dir(p1), filepath.Join(vault, "assets"))
	}
}

func TestAssetRelAndResolve(t *testing.T) {
	assetAbs := "/v/assets/img.png"
	rel := assetRelPath("/v/sub", assetAbs)
	if rel != "../assets/img.png" {
		t.Errorf("assetRelPath = %q, want ../assets/img.png", rel)
	}
	if got := resolveAssetPath("/v/sub", rel); got != assetAbs {
		t.Errorf("resolveAssetPath = %q, want %q", got, assetAbs)
	}
}
