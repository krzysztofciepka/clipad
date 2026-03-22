package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var previewStyle = lipgloss.NewStyle().Padding(0, 1)

func renderMarkdown(content string, width int) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4), // account for padding
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(content)
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
