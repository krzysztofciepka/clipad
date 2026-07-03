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

// TestPreviewViewportInjectsImageThroughGlamour proves the NUL sentinel used
// by stripImagesForGlamour survives a real glamour render (not just the
// hand-rolled injection above) and is correctly replaced by the image chip,
// with no raw sentinel or link text leaking into the final viewport content.
func TestPreviewViewportInjectsImageThroughGlamour(t *testing.T) {
	md := "# T\n\n![](assets/pic.png)\n\nbody"
	vp, err := newPreviewViewport(md, 80, 20, newImageRegistry(), false, t.TempDir())
	if err != nil {
		t.Fatalf("newPreviewViewport error: %v", err)
	}
	got := vp.View()
	if !strings.Contains(got, "🖼 image (pic.png)") {
		t.Errorf("rendered viewport missing image chip:\n%s", got)
	}
	if strings.Contains(got, "\x00IMG") {
		t.Errorf("rendered viewport leaked raw sentinel:\n%s", got)
	}
	if strings.Contains(got, "assets/pic.png") {
		t.Errorf("rendered viewport leaked raw link text:\n%s", got)
	}
}
