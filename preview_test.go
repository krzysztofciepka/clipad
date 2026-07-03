package main

import (
	"strings"
	"testing"
)

func TestStripAndInjectPreviewImages(t *testing.T) {
	md := "# Title\n\n![](assets/pic.png)\n\ntext"
	clean, els := stripImagesForGlamour(md)
	if len(els) != 1 || els[0] != "assets/pic.png" {
		t.Fatalf("elements = %v", els)
	}
	if strings.Contains(clean, "assets/pic.png") {
		t.Errorf("clean markdown still contains the link: %q", clean)
	}
	if !strings.Contains(clean, "\x00IMG0\x00") {
		t.Errorf("sentinel missing: %q", clean)
	}
	// Injection (fallback chip path).
	out := injectPreviewImages(clean, els, newImageRegistry(), false, "/note")
	if !strings.Contains(out, "🖼 image (pic.png)") {
		t.Errorf("chip not injected: %q", out)
	}
	if strings.Contains(out, "\x00IMG0\x00") {
		t.Errorf("sentinel not replaced: %q", out)
	}
}
