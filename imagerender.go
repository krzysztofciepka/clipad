package main

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

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

// buildTransmitSequence returns a kitty graphics APC sequence that transmits
// the given PNG data and creates a virtual placement (a=T,U=1) sized cols x
// rows, chunked at kitty.MaxChunkSize bytes per the kitty graphics protocol.
func buildTransmitSequence(id, cols, rows int, png []byte) string {
	b64 := base64.StdEncoding.EncodeToString(png)
	chunks := chunkString(b64, kitty.MaxChunkSize)
	var sb strings.Builder
	for i, chunk := range chunks {
		var opts []string
		if i == 0 {
			o := kitty.Options{
				Action:           kitty.TransmitAndPut,
				Format:           kitty.PNG,
				ID:               id,
				Columns:          cols,
				Rows:             rows,
				VirtualPlacement: true,
				Quite:            2,
			}
			opts = o.Options()
		}
		if len(chunks) > 1 {
			if i == len(chunks)-1 {
				opts = append(opts, "m=0")
			} else {
				opts = append(opts, "m=1")
			}
		}
		sb.WriteString(ansi.KittyGraphics([]byte(chunk), opts...))
	}
	return sb.String()
}

// chunkString splits s into chunks of at most size bytes each.
func chunkString(s string, size int) []string {
	if size <= 0 || len(s) <= size {
		return []string{s}
	}
	var out []string
	for len(s) > size {
		out = append(out, s[:size])
		s = s[size:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

// buildPlaceholderBlock returns one string per row of Unicode placeholder
// cells encoding id (as a 24-bit foreground color) plus row/col diacritics,
// per the kitty graphics protocol's Unicode placeholder scheme.
func buildPlaceholderBlock(id, cols, rows int) []string {
	r := byte((id >> 16) & 0xff)
	g := byte((id >> 8) & 0xff)
	b := byte(id & 0xff)
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	out := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		var sb strings.Builder
		sb.WriteString(fg)
		for col := 0; col < cols; col++ {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(kitty.Diacritic(row))
			sb.WriteRune(kitty.Diacritic(col))
		}
		sb.WriteString("\x1b[39m")
		out = append(out, sb.String())
	}
	return out
}
