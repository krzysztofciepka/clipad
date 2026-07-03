package main

import "strings"

// detectKittyGraphics reports whether the terminal supports the kitty graphics
// protocol, based on environment hints. This is a heuristic that avoids a
// mid-startup terminal query race (see setDarkBackground in main.go).
func detectKittyGraphics(getenv func(string) string) bool {
	if getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	term := strings.ToLower(getenv("TERM"))
	if strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") || strings.Contains(term, "wezterm") {
		return true
	}
	prog := strings.ToLower(getenv("TERM_PROGRAM"))
	switch prog {
	case "ghostty", "wezterm", "kitty":
		return true
	}
	return false
}

const (
	defaultCellW = 8
	defaultCellH = 16
	maxImageRows = 12
	maxImageCols = 64
)

func imageRenderSize(imgW, imgH, cellW, cellH, maxCols, maxRows int) (int, int) {
	if imgW <= 0 || imgH <= 0 {
		return 1, 1
	}
	if cellW <= 0 {
		cellW = defaultCellW
	}
	if cellH <= 0 {
		cellH = defaultCellH
	}
	cols := (imgW + cellW - 1) / cellW
	rows := (imgH + cellH - 1) / cellH
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	// Scale down preserving aspect ratio if either dimension exceeds its cap.
	if cols > maxCols || rows > maxRows {
		scaleCols := float64(maxCols) / float64(cols)
		scaleRows := float64(maxRows) / float64(rows)
		scale := scaleCols
		if scaleRows < scale {
			scale = scaleRows
		}
		cols = int(float64(cols) * scale)
		rows = int(float64(rows) * scale)
		if cols < 1 {
			cols = 1
		}
		if rows < 1 {
			rows = 1
		}
	}
	return cols, rows
}
