package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var previewStyle = lipgloss.NewStyle().Padding(0, 1)

var (
	cachedRenderer      *glamour.TermRenderer
	cachedRendererWidth int
)

func getRenderer(width int) (*glamour.TermRenderer, error) {
	if cachedRenderer != nil && cachedRendererWidth == width {
		return cachedRenderer, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
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

func newPreviewViewport(content string, width, height int) (viewport.Model, error) {
	rendered, err := renderMarkdown(content, width)
	if err != nil {
		return viewport.Model{}, err
	}
	vp := viewport.New(width-2, height)
	vp.SetContent(rendered)
	return vp, nil
}
