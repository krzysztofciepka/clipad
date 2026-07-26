package main

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
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

type imageRegistry struct {
	ids         map[string]int
	transmitted map[string]bool
	dims        map[string][2]int
	nextID      int
}

func newImageRegistry() *imageRegistry {
	return &imageRegistry{
		ids:         map[string]int{},
		transmitted: map[string]bool{},
		dims:        map[string][2]int{},
		nextID:      1,
	}
}

// dimensionsFor returns the pixel size of the image at abs, caching successful
// decodes for the session. The layout sizes every image element on every
// keystroke, so without this an image-heavy note re-reads each file from disk
// several times per frame. Assets are content-addressed, so a cached path
// keeps its dimensions; a hand-written link to a file edited in place outside
// clipad keeps its old size until restart, matching how the transmit-once
// bookkeeping already behaves.
func (r *imageRegistry) dimensionsFor(abs string) (int, int, error) {
	if wh, ok := r.dims[abs]; ok {
		return wh[0], wh[1], nil
	}
	w, h, err := imageDimensions(abs)
	if err != nil {
		return 0, 0, err
	}
	r.dims[abs] = [2]int{w, h}
	return w, h, nil
}

func (r *imageRegistry) idFor(hash string) int {
	if id, ok := r.ids[hash]; ok {
		return id
	}
	id := r.nextID
	r.ids[hash] = id
	r.nextID++
	return id
}

func (r *imageRegistry) markTransmitted(hash string) bool {
	if r.transmitted[hash] {
		return false
	}
	r.transmitted[hash] = true
	return true
}

// imageDimensions returns the pixel width/height of the image at path,
// decoded from its header only. Overridable in tests.
var imageDimensions = func(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// imageChip renders the fallback single-line representation of an image
// element for terminals without kitty graphics support.
func imageChip(target string) string {
	return "🖼 image (" + filepath.Base(target) + ")"
}

// imageBlockSize returns the cell dimensions an image element occupies and
// whether the kitty path applies (false means the one-line chip fallback).
// It reads only cached/decoded image headers and never consumes the
// registry's transmit-once flag, so the layout can size a block without
// affecting what the renderer later emits.
func imageBlockSize(reg *imageRegistry, kittyOK bool, noteDir, target string, maxCols int) (cols, rows int, ok bool) {
	if !kittyOK {
		return 0, 1, false
	}
	abs := resolveAssetPath(noteDir, target)
	var (
		w, h int
		err  error
	)
	if reg != nil {
		w, h, err = reg.dimensionsFor(abs)
	} else {
		w, h, err = imageDimensions(abs)
	}
	if err != nil {
		return 0, 1, false
	}
	if maxCols > maxImageCols {
		maxCols = maxImageCols
	}
	cols, rows = imageRenderSize(w, h, defaultCellW, defaultCellH, maxCols, maxImageRows)
	return cols, rows, true
}

// renderImageElement returns the display rows for an image-element line and
// how many terminal rows they occupy. On the kitty path it returns
// placeholder rows (the first row prefixed with the transmit sequence the
// first time the asset is rendered in this session). On the fallback path it
// returns a single chip row.
func renderImageElement(reg *imageRegistry, kittyOK bool, noteDir, target string, maxCols int) ([]string, int) {
	return renderImageElementFrom(reg, kittyOK, noteDir, target, maxCols, 0)
}

// renderImageElementFrom is renderImageElement starting at block row `from`,
// used when the top of an image block has scrolled off the editor viewport.
// The transmit sequence stays attached to the first row actually returned,
// so a partially scrolled image is still transmitted rather than rendering
// as blank placeholder cells.
func renderImageElementFrom(reg *imageRegistry, kittyOK bool, noteDir, target string, maxCols, from int) ([]string, int) {
	cols, rows, ok := imageBlockSize(reg, kittyOK, noteDir, target, maxCols)
	if !ok {
		if from > 0 {
			return nil, 0
		}
		return []string{imageChip(target)}, 1
	}
	abs := resolveAssetPath(noteDir, target)
	data, _ := os.ReadFile(abs)     // best-effort; dimensions already validated above
	hash := assetFilename(data, "") // stable per content; date irrelevant for keying
	id := reg.idFor(hash)
	block := buildPlaceholderBlock(id, cols, rows)
	if from >= len(block) {
		return nil, 0
	}
	block = block[from:]
	if reg.markTransmitted(hash) {
		block[0] = buildTransmitSequence(id, cols, rows, data) + block[0]
	}
	return block, len(block)
}
