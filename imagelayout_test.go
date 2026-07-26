package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// stubImageDimensions makes imageDimensions report a fixed size for any path,
// so layout tests don't need real image files on disk.
func stubImageDimensions(t *testing.T, w, h int) {
	t.Helper()
	orig := imageDimensions
	imageDimensions = func(string) (int, int, error) { return w, h, nil }
	t.Cleanup(func() { imageDimensions = orig })
}

func kittyLayout() *imageLayout {
	return &imageLayout{reg: newImageRegistry(), kitty: true, noteDir: "/notes"}
}

// A 320x160 image at the default 8x16 cell size is 40x10 cells, so an image
// element line must occupy 10 visual rows, not 1.
func TestWrapContentImageElementOccupiesBlockRows(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nbelow"

	rows := wrapContent(content, 60, kittyLayout())

	var imageRows int
	for _, r := range rows {
		if r.line == 1 {
			imageRows++
		}
	}
	if imageRows != 10 {
		t.Errorf("image element occupies %d visual rows, want 10", imageRows)
	}
	if len(rows) != 12 {
		t.Errorf("total visual rows = %d, want 12 (1 + 10 + 1)", len(rows))
	}
}

// Each row of the block must record its index within the block, so a
// partially scrolled image can be sliced correctly.
func TestWrapContentImageRowsCarryBlockIndex(t *testing.T) {
	stubImageDimensions(t, 320, 160)

	rows := wrapContent("![](assets/a.png)", 60, kittyLayout())

	for i, r := range rows {
		if !r.image {
			t.Fatalf("row %d not marked as an image row", i)
		}
		if r.imgRow != i {
			t.Errorf("row %d has imgRow %d, want %d", i, r.imgRow, i)
		}
	}
}

// Without kitty support the element renders as a one-line chip, so the
// layout must stay exactly as it was before image support existed.
func TestWrapContentImageChipOccupiesOneRow(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nbelow"

	rows := wrapContent(content, 60, &imageLayout{reg: newImageRegistry(), noteDir: "/notes"})

	if len(rows) != 3 {
		t.Errorf("fallback chip layout has %d rows, want 3", len(rows))
	}
}

// The regression: text below a tall image must be clickable. Before the fix
// the click landed (imageRows-1) logical lines too far down.
func TestMousePosBelowImageMapsToClickedLine(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nAAA\nBBB\nCCC"
	img := kittyLayout()

	// "AAA" is drawn on visual row 11: row 0 is "top", rows 1-10 are the image.
	line, _ := mousePosToEditorCursor(content, 0, 8, 11, 2, 60, img)

	if line != 2 {
		t.Errorf("click on AAA's visual row maps to line %d, want 2", line)
	}
}

// Clicking inside the image block puts the cursor at the start of the
// element's line — never inside the link text, which is atomic.
func TestMousePosInsideImageBlockLandsAtElementStart(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nAAA"

	line, col := mousePosToEditorCursor(content, 0, 20, 5, 2, 60, kittyLayout())

	if line != 1 || col != 0 {
		t.Errorf("click inside image block = (%d, %d), want (1, 0)", line, col)
	}
}

// atomicCol parks the cursor at end-of-line when moving right onto an
// element; that position must still resolve to the block's first row rather
// than falling through to row 0.
func TestCursorVisualRowOnImageElementIsBlockTop(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nAAA"
	endCol := len([]rune("![](assets/a.png)"))

	got := cursorVisualRow(content, 1, endCol, 60, kittyLayout())

	if got != 1 {
		t.Errorf("cursorVisualRow at end of image line = %d, want 1", got)
	}
}

func TestCursorVisualRowBelowImageAccountsForBlock(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nAAA"

	got := cursorVisualRow(content, 2, 0, 60, kittyLayout())

	if got != 11 {
		t.Errorf("cursorVisualRow on the line after the image = %d, want 11", got)
	}
}

// When the top of an image block is scrolled off, the remaining rows must
// still carry the transmit sequence — otherwise the terminal never receives
// the image data and the placeholders render as blanks.
func TestRenderImageElementFromKeepsTransmitOnFirstRow(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	reg := newImageRegistry()

	block, n := renderImageElementFrom(reg, true, "/notes", "assets/a.png", 64, 3)

	if n != 7 || len(block) != 7 {
		t.Fatalf("block from row 3 has %d rows (n=%d), want 7", len(block), n)
	}
	if !strings.Contains(block[0], "\x1b_G") {
		t.Errorf("first emitted row is missing the kitty transmit sequence")
	}
}

// The layout sizes every image element in the document, and runs on every
// keystroke (render, scroll sync, click mapping). Decoding each image header
// from disk every time makes typing in an image-heavy note slow, so the
// registry caches dimensions per session.
func TestImageDimensionsDecodedOncePerAsset(t *testing.T) {
	calls := 0
	orig := imageDimensions
	imageDimensions = func(string) (int, int, error) { calls++; return 320, 160, nil }
	t.Cleanup(func() { imageDimensions = orig })

	content := "![](assets/a.png)\ntext\n![](assets/b.png)"
	img := kittyLayout()

	for i := 0; i < 5; i++ {
		wrapContent(content, 60, img)
	}

	if calls != 2 {
		t.Errorf("decoded image headers %d times for 2 distinct assets, want 2", calls)
	}
}

// The editor render and the click mapping must agree row-for-row: whatever
// visual row shows a given logical line is the row a click on it resolves to.
func TestEditorRenderAgreesWithClickMapping(t *testing.T) {
	stubImageDimensions(t, 320, 160)
	content := "top\n![](assets/a.png)\nAAA\nBBB"

	e := newSelectableEditor()
	setEditorSize(&e, 62, 20)
	e.SetValue(content)
	e.SetImageContext(newImageRegistry(), true, "/notes")

	drawn := strings.Split(strings.TrimRight(e.render(), "\n"), "\n")
	for i, row := range drawn {
		if !strings.Contains(ansi.Strip(row), "AAA") {
			continue
		}
		line, _ := mousePosToEditorCursor(content, 0, 8, i, 2, e.Width(), e.imageLayout())
		if line != 2 {
			t.Errorf("AAA drawn on visual row %d, but a click there maps to line %d, want 2", i, line)
		}
		return
	}
	t.Fatalf("AAA was not drawn:\n%s", strings.Join(drawn, "\n"))
}
