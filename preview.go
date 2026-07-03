package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var previewStyle = lipgloss.NewStyle().Padding(0, 1)

// darkBackground records whether the user's terminal has a dark
// background. It is set once at startup by main.go via setDarkBackground
// (before tea.Program claims stdin), so glamour can pick a fixed style
// without doing its own OSC 11 query mid-session.
var darkBackground = true

func setDarkBackground(dark bool) {
	darkBackground = dark
	cachedRenderer = nil
	cachedRendererWidth = 0
}

var (
	cachedRenderer      *glamour.TermRenderer
	cachedRendererWidth int
)

func getRenderer(width int) (*glamour.TermRenderer, error) {
	if cachedRenderer != nil && cachedRendererWidth == width {
		return cachedRenderer, nil
	}
	style := "dark"
	if !darkBackground {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return nil, err
	}
	cachedRenderer = r
	cachedRendererWidth = width
	return r, nil
}

func renderMarkdown(content string, width int) (string, error) {
	r, err := getRenderer(width)
	if err != nil {
		return "", err
	}
	return r.Render(content)
}

// stripImagesForGlamour replaces each lone image-element line in md with a
// unique sentinel token on its own line, so glamour never gets a chance to
// turn the image into alt-text. It returns the sanitized markdown along with
// the ordered list of image targets, one per sentinel index.
func stripImagesForGlamour(md string) (string, []string) {
	lines := strings.Split(md, "\n")
	var els []string
	for i, ln := range lines {
		if target, ok := parseImageElement(ln); ok {
			lines[i] = fmt.Sprintf("\x00IMG%d\x00", len(els))
			els = append(els, target)
		}
	}
	return strings.Join(lines, "\n"), els
}

// injectPreviewImages replaces each sentinel token in rendered (glamour's
// output) with the display rows for the corresponding image element: either
// kitty placeholder rows or a fallback chip, depending on kitty/noteDir/reg.
func injectPreviewImages(rendered string, els []string, reg *imageRegistry, kitty bool, noteDir string) string {
	for i, target := range els {
		sentinel := fmt.Sprintf("\x00IMG%d\x00", i)
		block, _ := renderImageElement(reg, kitty, noteDir, target, maxImageCols)
		rendered = strings.Replace(rendered, sentinel, strings.Join(block, "\n"), 1)
	}
	return rendered
}

func newPreviewViewport(content string, width, height int, reg *imageRegistry, kitty bool, noteDir string) (viewport.Model, error) {
	clean, els := stripImagesForGlamour(content)
	rendered, err := renderMarkdown(clean, width)
	if err != nil {
		return viewport.Model{}, err
	}
	rendered = injectPreviewImages(rendered, els, reg, kitty, noteDir)
	vp := viewport.New(width-2, height)
	vp.SetContent(rendered)
	return vp, nil
}
