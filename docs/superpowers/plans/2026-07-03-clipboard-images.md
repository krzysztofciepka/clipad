# Clipboard Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user paste a clipboard image into a note with Ctrl+V, stored as a content-addressed file under `assets/` but rendered inline as real pixels in the editor and preview, behaving as one atomic element that can be deleted, cut, or copied (copy/cut of a lone image puts the real bytes back on the clipboard).

**Architecture:** Three new files carry the new concerns — `imageclip.go` (system-clipboard image I/O by shelling out to `wl-paste`/`xclip`/`wl-copy`), `imageelement.go` (detect/parse the `![](assets/…)` element line and manage content-addressed asset files), and `imagerender.go` (kitty graphics: capability detection, transmit + Unicode-placeholder builders, sizing, per-session image registry). Existing files get targeted edits: `main.go` (startup capability probe), `model.go` (Ctrl+V/C/X routing), `selection.go` (editor render splice + atomic navigation), `preview.go` (glamour post-process). Rendering uses the kitty **Unicode placeholder** mechanism via `github.com/charmbracelet/x/ansi/kitty`, so images live in the text grid and scroll/reflow under Bubble Tea's full-frame redraw.

**Tech Stack:** Go 1.26.1, Bubble Tea, `github.com/charmbracelet/x/ansi` + its `/kitty` subpackage (already a direct dependency), `os/exec` for clipboard tools, `crypto/sha256`, `encoding/base64`.

## Global Constraints

- Module `clipad`, everything in `package main`; Go `1.26.1`.
- **No new Go dependencies.** Use only what is already in `go.mod` (notably `github.com/charmbracelet/x/ansi` and its `/kitty` subpackage). Clipboard image support is provided by shelling out to system tools (`wl-clipboard` / `xclip`) — a runtime requirement, not a Go dependency.
- Assets live in `<vault>/assets/`; filenames are content-addressed: `img-<YYYY-MM-DD>-<first8hexOfSHA256>.png`. Links in notes are written **relative to the note file's directory**.
- Kitty graphics protocol only. When unsupported (or the asset/tool is missing), fall back to a text chip `🖼 image (<name>)`; never crash and never lose data.
- Existing text-paste / text-copy / text-cut behavior must be unchanged when the clipboard holds no image and the selection is not a lone image element.
- Every clipad release must include a `linux/amd64` binary asset (project release convention).
- Run the full test suite with `go test ./...` from `/home/kc/repos/clipad`.

---

### Task 1: Image-element line detection & parsing

**Files:**
- Create: `imageelement.go`
- Test: `imageelement_test.go`

**Interfaces:**
- Produces: `func parseImageElement(line string) (target string, ok bool)` — returns the link target and true when `line` is exactly a markdown image whose target has an image extension; false otherwise.
- Produces: `var imageExtensions = map[string]bool{...}` (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`).

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestParseImageElement -v`
Expected: FAIL — `undefined: parseImageElement`.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

// imageElementRe matches a whole line that is exactly a markdown image:
// optional surrounding whitespace, ![any alt](target), nothing else.
var imageElementRe = regexp.MustCompile(`^\s*!\[[^\]]*\]\(([^)]+)\)\s*$`)

// parseImageElement reports whether line is a lone markdown image whose target
// has a recognized image extension, returning the target path.
func parseImageElement(line string) (string, bool) {
	m := imageElementRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	target := m[1]
	if !imageExtensions[strings.ToLower(filepath.Ext(target))] {
		return "", false
	}
	return target, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestParseImageElement -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imageelement.go imageelement_test.go
git commit -m "feat(images): detect markdown image-element lines"
```

---

### Task 2: Content-addressed asset storage & path helpers

**Files:**
- Modify: `imageelement.go`
- Test: `imageelement_test.go`

**Interfaces:**
- Consumes: `copyFile`/`uniquePath` exist in `clipboard.go` (not needed here — content addressing replaces uniqueness).
- Produces:
  - `func assetFilename(data []byte, date string) string` → `img-<date>-<short8>.png`.
  - `func saveAsset(vault string, data []byte, date string) (absPath string, err error)` — writes `<vault>/assets/<name>` if absent (idempotent by content), returns absolute path.
  - `func assetRelPath(noteDir, assetAbs string) string` — asset path relative to the note's directory (forward-slash, for the markdown link).
  - `func resolveAssetPath(noteDir, target string) string` — absolute path of a link target relative to the note's directory.

- [ ] **Step 1: Write the failing test**

```go
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
	vault := "/v"
	assetAbs := "/v/assets/img.png"
	rel := assetRelPath("/v/sub", assetAbs)
	if rel != "../assets/img.png" {
		t.Errorf("assetRelPath = %q, want ../assets/img.png", rel)
	}
	if got := resolveAssetPath("/v/sub", rel); got != assetAbs {
		t.Errorf("resolveAssetPath = %q, want %q", got, assetAbs)
	}
}
```

Add imports `os`, `path/filepath` to the test file if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestAssetFilename|TestSaveAsset|TestAssetRel' -v`
Expected: FAIL — undefined `assetFilename`, `saveAsset`, `assetRelPath`, `resolveAssetPath`.

- [ ] **Step 3: Write minimal implementation**

Append to `imageelement.go` (add imports `crypto/sha256`, `encoding/hex`, `fmt`, `os`):

```go
func assetFilename(data []byte, date string) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("img-%s-%s.png", date, hex.EncodeToString(sum[:])[:8])
}

func saveAsset(vault string, data []byte, date string) (string, error) {
	dir := filepath.Join(vault, "assets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs := filepath.Join(dir, assetFilename(data, date))
	if _, err := os.Stat(abs); err == nil {
		return abs, nil // already stored (content-addressed)
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", err
	}
	return abs, nil
}

func assetRelPath(noteDir, assetAbs string) string {
	rel, err := filepath.Rel(noteDir, assetAbs)
	if err != nil {
		return assetAbs
	}
	return filepath.ToSlash(rel)
}

func resolveAssetPath(noteDir, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(noteDir, filepath.FromSlash(target))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestAssetFilename|TestSaveAsset|TestAssetRel' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imageelement.go imageelement_test.go
git commit -m "feat(images): content-addressed asset storage and path helpers"
```

---

### Task 3: Clipboard command construction (pure)

**Files:**
- Create: `imageclip.go`
- Test: `imageclip_test.go`

**Interfaces:**
- Produces:
  - `type clipEnv int` with `clipWayland`, `clipX11`, `clipNone`.
  - `func detectClipEnv(getenv func(string) string) clipEnv`.
  - `func probeImageCmd(env clipEnv) (name string, args []string)` — command to list clipboard MIME types.
  - `func readImageCmd(env clipEnv) (name string, args []string)` — command to read `image/png` bytes to stdout.
  - `func writeImageCmd(env clipEnv) (name string, args []string)` — command that reads PNG bytes on stdin and sets the clipboard.
  - `func probeIndicatesImage(env clipEnv, out []byte) bool` — true when probe output advertises an image type.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestDetectClipEnv(t *testing.T) {
	way := func(k string) string { if k == "WAYLAND_DISPLAY" { return "wayland-0" }; return "" }
	x11 := func(k string) string { if k == "DISPLAY" { return ":0" }; return "" }
	none := func(string) string { return "" }
	if detectClipEnv(way) != clipWayland {
		t.Error("want wayland")
	}
	if detectClipEnv(x11) != clipX11 {
		t.Error("want x11")
	}
	if detectClipEnv(none) != clipNone {
		t.Error("want none")
	}
}

func TestClipCommands(t *testing.T) {
	n, a := readImageCmd(clipWayland)
	if n != "wl-paste" || a[0] != "--type" || a[1] != "image/png" {
		t.Errorf("wayland read = %q %v", n, a)
	}
	n, a = readImageCmd(clipX11)
	if n != "xclip" || a[len(a)-1] != "image/png" {
		t.Errorf("x11 read = %q %v", n, a)
	}
	n, _ = writeImageCmd(clipWayland)
	if n != "wl-copy" {
		t.Errorf("wayland write = %q", n)
	}
}

func TestProbeIndicatesImage(t *testing.T) {
	if !probeIndicatesImage(clipWayland, []byte("text/plain\nimage/png\n")) {
		t.Error("wayland: want image detected")
	}
	if probeIndicatesImage(clipWayland, []byte("text/plain\n")) {
		t.Error("wayland: want no image")
	}
	if !probeIndicatesImage(clipX11, []byte("TARGETS\nimage/png\nUTF8_STRING\n")) {
		t.Error("x11: want image detected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestDetectClipEnv|TestClipCommands|TestProbeIndicatesImage' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import (
	"os"
	"strings"
)

type clipEnv int

const (
	clipNone clipEnv = iota
	clipWayland
	clipX11
)

func detectClipEnv(getenv func(string) string) clipEnv {
	if getenv("WAYLAND_DISPLAY") != "" {
		return clipWayland
	}
	if getenv("DISPLAY") != "" {
		return clipX11
	}
	return clipNone
}

func probeImageCmd(env clipEnv) (string, []string) {
	switch env {
	case clipWayland:
		return "wl-paste", []string{"--list-types"}
	case clipX11:
		return "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}
	}
	return "", nil
}

func readImageCmd(env clipEnv) (string, []string) {
	switch env {
	case clipWayland:
		return "wl-paste", []string{"--type", "image/png", "--no-newline"}
	case clipX11:
		return "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}
	}
	return "", nil
}

func writeImageCmd(env clipEnv) (string, []string) {
	switch env {
	case clipWayland:
		return "wl-copy", []string{"--type", "image/png"}
	case clipX11:
		return "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}
	}
	return "", nil
}

func probeIndicatesImage(env clipEnv, out []byte) bool {
	return strings.Contains(string(out), "image/png")
}

// currentClipEnv is a convenience wrapper over the real environment.
func currentClipEnv() clipEnv { return detectClipEnv(os.Getenv) }
```

Note: `readImageCmd(clipWayland)` returns args `["--type","image/png","--no-newline"]`; the test checks `a[0]`/`a[1]` only, so it stays valid.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestDetectClipEnv|TestClipCommands|TestProbeIndicatesImage' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imageclip.go imageclip_test.go
git commit -m "feat(images): clipboard image command construction"
```

---

### Task 4: Clipboard exec wrappers (injectable runner)

**Files:**
- Modify: `imageclip.go`
- Test: `imageclip_test.go`

**Interfaces:**
- Produces:
  - `var runCapture func(name string, args ...string) ([]byte, error)` (default: `exec.Command(...).Output()`).
  - `var runWithStdin func(stdin []byte, name string, args ...string) error` (default: pipes stdin into `exec.Command`).
  - `func clipboardHasImage(env clipEnv) bool`
  - `func readClipboardImage(env clipEnv) ([]byte, error)`
  - `func writeClipboardImage(env clipEnv, data []byte) error`
  - `func imageToolAvailable(env clipEnv) bool` (uses `exec.LookPath` on the read tool).

- [ ] **Step 1: Write the failing test**

```go
func TestClipboardHasImageUsesRunner(t *testing.T) {
	old := runCapture
	defer func() { runCapture = old }()
	runCapture = func(name string, args ...string) ([]byte, error) {
		return []byte("text/plain\nimage/png\n"), nil
	}
	if !clipboardHasImage(clipWayland) {
		t.Error("want image detected via injected runner")
	}
	runCapture = func(name string, args ...string) ([]byte, error) {
		return []byte("text/plain\n"), nil
	}
	if clipboardHasImage(clipWayland) {
		t.Error("want no image")
	}
}

func TestWriteClipboardImageUsesStdin(t *testing.T) {
	old := runWithStdin
	defer func() { runWithStdin = old }()
	var gotStdin []byte
	var gotName string
	runWithStdin = func(stdin []byte, name string, args ...string) error {
		gotStdin = stdin
		gotName = name
		return nil
	}
	_ = writeClipboardImage(clipWayland, []byte("BYTES"))
	if gotName != "wl-copy" || string(gotStdin) != "BYTES" {
		t.Errorf("write = %q stdin=%q", gotName, gotStdin)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestClipboardHasImageUsesRunner|TestWriteClipboardImageUsesStdin' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write minimal implementation**

Append to `imageclip.go` (add imports `bytes`, `os/exec`, `fmt`):

```go
var runCapture = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var runWithStdin = func(stdin []byte, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	return cmd.Run()
}

func imageToolAvailable(env clipEnv) bool {
	name, _ := readImageCmd(env)
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func clipboardHasImage(env clipEnv) bool {
	name, args := probeImageCmd(env)
	if name == "" {
		return false
	}
	out, err := runCapture(name, args...)
	if err != nil {
		return false
	}
	return probeIndicatesImage(env, out)
}

func readClipboardImage(env clipEnv) ([]byte, error) {
	name, args := readImageCmd(env)
	if name == "" {
		return nil, fmt.Errorf("no clipboard tool for this environment")
	}
	return runCapture(name, args...)
}

func writeClipboardImage(env clipEnv, data []byte) error {
	name, args := writeImageCmd(env)
	if name == "" {
		return fmt.Errorf("no clipboard tool for this environment")
	}
	return runWithStdin(data, name, args...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestClipboardHasImageUsesRunner|TestWriteClipboardImageUsesStdin' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imageclip.go imageclip_test.go
git commit -m "feat(images): clipboard image exec wrappers with injectable runner"
```

---

### Task 5: Kitty capability detection + startup wiring

**Files:**
- Create: `imagerender.go`
- Modify: `main.go:198` (near `setDarkBackground`), `model.go:82-130` (add field), `model.go:219` (`newModel`)
- Test: `imagerender_test.go`

**Interfaces:**
- Produces: `func detectKittyGraphics(getenv func(string) string) bool`.
- Produces: model field `kittyImages bool`, set at startup.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestDetectKittyGraphics(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if !detectKittyGraphics(env(map[string]string{"KITTY_WINDOW_ID": "1"})) {
		t.Error("kitty window id should enable")
	}
	if !detectKittyGraphics(env(map[string]string{"TERM": "xterm-kitty"})) {
		t.Error("xterm-kitty TERM should enable")
	}
	if !detectKittyGraphics(env(map[string]string{"TERM_PROGRAM": "ghostty"})) {
		t.Error("ghostty should enable")
	}
	if !detectKittyGraphics(env(map[string]string{"TERM_PROGRAM": "WezTerm"})) {
		t.Error("wezterm should enable")
	}
	if detectKittyGraphics(env(map[string]string{"TERM": "xterm-256color"})) {
		t.Error("plain xterm should not enable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestDetectKittyGraphics -v`
Expected: FAIL — `undefined: detectKittyGraphics`.

- [ ] **Step 3: Write minimal implementation**

Create `imagerender.go`:

```go
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
```

Add the model field in `model.go` (in the `model struct`, near `errMsg string` around line 128):

```go
	kittyImages bool
```

In `newModel` (`model.go:219`), set it on the constructed model before returning (find the `m := model{...}` composite literal near line 285 and add `kittyImages: detectKittyGraphics(os.Getenv),` — `os` is already imported in model.go; if not, add it).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestDetectKittyGraphics -v`
Expected: PASS.

- [ ] **Step 5: Build to confirm wiring compiles**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add imagerender.go imagerender_test.go model.go
git commit -m "feat(images): kitty graphics capability detection"
```

---

### Task 6: Image render sizing math

**Files:**
- Modify: `imagerender.go`
- Test: `imagerender_test.go`

**Interfaces:**
- Produces: `func imageRenderSize(imgW, imgH, cellW, cellH, maxCols, maxRows int) (cols, rows int)` — number of terminal cells to occupy, preserving aspect ratio, clamped to `maxCols`/`maxRows`, always ≥ 1.
- Produces: `const defaultCellW = 8`, `const defaultCellH = 16` (fallback cell pixel size), `const maxImageRows = 12`, `const maxImageCols = 64`.

- [ ] **Step 1: Write the failing test**

```go
func TestImageRenderSize(t *testing.T) {
	// 800x400 image, 8x16 cells => 100 cols x 25 rows natural; clamp to 64x12.
	cols, rows := imageRenderSize(800, 400, 8, 16, 64, 12)
	if cols < 1 || rows < 1 {
		t.Fatalf("got %dx%d, want >=1", cols, rows)
	}
	if cols > 64 || rows > 12 {
		t.Errorf("not clamped: %dx%d", cols, rows)
	}
	// Aspect ratio preserved within rounding: natural cols/rows ~ 4:1.
	if rows != 12 && cols != 64 {
		t.Errorf("expected one dimension at the cap, got %dx%d", cols, rows)
	}
	// Tiny image stays tiny and never zero.
	c2, r2 := imageRenderSize(4, 8, 8, 16, 64, 12)
	if c2 < 1 || r2 < 1 {
		t.Errorf("tiny image => %dx%d, want >=1", c2, r2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestImageRenderSize -v`
Expected: FAIL — `undefined: imageRenderSize`.

- [ ] **Step 3: Write minimal implementation**

Append to `imagerender.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestImageRenderSize -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imagerender.go imagerender_test.go
git commit -m "feat(images): aspect-preserving render sizing"
```

---

### Task 7: Kitty transmit + Unicode-placeholder builders

**Files:**
- Modify: `imagerender.go`
- Test: `imagerender_test.go`

**Interfaces:**
- Consumes: `github.com/charmbracelet/x/ansi` (`ansi.KittyGraphics`) and `github.com/charmbracelet/x/ansi/kitty` (`kitty.Options`, `kitty.Placeholder`, `kitty.Diacritic`, `kitty.MaxChunkSize`, `kitty.PNG`, `kitty.TransmitAndPut`).
- Produces:
  - `func buildTransmitSequence(id, cols, rows int, png []byte) string` — kitty APC to transmit a PNG and create a virtual placement (`a=T,U=1`) sized `cols`×`rows`, chunked at `kitty.MaxChunkSize`.
  - `func buildPlaceholderBlock(id, cols, rows int) []string` — one string per row of placeholder cells encoding `id` (24-bit fg color) + row/col diacritics.

- [ ] **Step 1: Write the failing test**

```go
import (
	"strings"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestBuildPlaceholderBlock(t *testing.T) {
	rows := buildPlaceholderBlock(1, 2, 1)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	line := rows[0]
	// id=1 => 24-bit fg 0;0;1
	if !strings.HasPrefix(line, "\x1b[38;2;0;0;1m") {
		t.Errorf("missing fg color prefix: %q", line)
	}
	if !strings.HasSuffix(line, "\x1b[39m") {
		t.Errorf("missing fg reset suffix: %q", line)
	}
	if strings.Count(line, string(kitty.Placeholder)) != 2 {
		t.Errorf("want 2 placeholder cells, got %d", strings.Count(line, string(kitty.Placeholder)))
	}
	// First cell: placeholder + row diacritic(0) + col diacritic(0).
	firstCell := string(kitty.Placeholder) + string(kitty.Diacritic(0)) + string(kitty.Diacritic(0))
	if !strings.Contains(line, firstCell) {
		t.Errorf("first cell encoding missing in %q", line)
	}
}

func TestBuildTransmitSequenceSmall(t *testing.T) {
	seq := buildTransmitSequence(7, 3, 2, []byte("PNG"))
	if !strings.HasPrefix(seq, "\x1b_G") {
		t.Errorf("not an APC _G sequence: %q", seq)
	}
	for _, want := range []string{"i=7", "U=1", "a=T", "f=100", "c=3", "r=2"} {
		if !strings.Contains(seq, want) {
			t.Errorf("transmit seq missing %q: %q", want, seq)
		}
	}
	if !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("not terminated with ST: %q", seq)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestBuildPlaceholderBlock|TestBuildTransmitSequenceSmall' -v`
Expected: FAIL — undefined builders.

- [ ] **Step 3: Write minimal implementation**

Append to `imagerender.go` (add imports `encoding/base64`, `fmt`, `strings`, `github.com/charmbracelet/x/ansi`, `github.com/charmbracelet/x/ansi/kitty`):

```go
func buildTransmitSequence(id, cols, rows int, png []byte) string {
	b64 := base64.StdEncoding.EncodeToString(png)
	var sb strings.Builder
	first := true
	for len(b64) > 0 {
		n := kitty.MaxChunkSize
		if n > len(b64) {
			n = len(b64)
		}
		chunk := b64[:n]
		b64 = b64[n:]
		last := len(b64) == 0
		var opts []string
		if first {
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
			first = false
		}
		if !last {
			opts = append(opts, "m=1")
		} else if !first || strings.Contains(strings.Join(opts, ","), "m=1") {
			// Only emit m=0 when the payload was actually chunked.
		}
		if !last {
			// already appended m=1
		}
		sb.WriteString(ansi.KittyGraphics([]byte(chunk), opts...))
	}
	return sb.String()
}
```

Replace the muddled chunk-flag logic above with this clean version (use this as the final body):

```go
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
```

Delete the first, muddled `buildTransmitSequence` draft — keep only the clean version plus `chunkString` and `buildPlaceholderBlock`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestBuildPlaceholderBlock|TestBuildTransmitSequenceSmall' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imagerender.go imagerender_test.go
git commit -m "feat(images): kitty transmit and unicode-placeholder builders"
```

---

### Task 8: Per-session image registry (transmit-once + id allocation)

**Files:**
- Modify: `imagerender.go`
- Test: `imagerender_test.go`

**Interfaces:**
- Produces:
  - `type imageRegistry struct{ ... }` with `ids map[string]int`, `transmitted map[string]bool`, `nextID int`.
  - `func newImageRegistry() *imageRegistry`.
  - `func (r *imageRegistry) idFor(hash string) int` — stable per-hash id, allocated on first use starting at 1.
  - `func (r *imageRegistry) markTransmitted(hash string) (firstTime bool)` — returns true only the first time for a hash.

- [ ] **Step 1: Write the failing test**

```go
func TestImageRegistry(t *testing.T) {
	r := newImageRegistry()
	id1 := r.idFor("aaa")
	id2 := r.idFor("bbb")
	if id1 == id2 || id1 < 1 || id2 < 1 {
		t.Errorf("ids not distinct/positive: %d %d", id1, id2)
	}
	if r.idFor("aaa") != id1 {
		t.Error("id not stable for same hash")
	}
	if !r.markTransmitted("aaa") {
		t.Error("first markTransmitted should be true")
	}
	if r.markTransmitted("aaa") {
		t.Error("second markTransmitted should be false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestImageRegistry -v`
Expected: FAIL — undefined `newImageRegistry`.

- [ ] **Step 3: Write minimal implementation**

Append to `imagerender.go`:

```go
type imageRegistry struct {
	ids         map[string]int
	transmitted map[string]bool
	nextID      int
}

func newImageRegistry() *imageRegistry {
	return &imageRegistry{ids: map[string]int{}, transmitted: map[string]bool{}, nextID: 1}
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestImageRegistry -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imagerender.go imagerender_test.go
git commit -m "feat(images): per-session image registry"
```

---

### Task 9: Render an image element to terminal rows

**Files:**
- Modify: `imagerender.go`
- Test: `imagerender_test.go`

**Interfaces:**
- Consumes: `parseImageElement`, `resolveAssetPath`, `imageRenderSize`, `buildTransmitSequence`, `buildPlaceholderBlock`, `imageRegistry`, `assetFilename` inputs.
- Produces:
  - `func renderImageElement(reg *imageRegistry, kitty bool, noteDir, target string, maxCols int) (lines []string, height int)` — returns the display rows for an image-element line and how many terminal rows they occupy. On kitty path, returns placeholder rows (first row prefixed with the transmit sequence the first time). On fallback, returns a single chip row `🖼 image (<name>)`.
- Uses image dimensions via a package var `imageDimensions func(path string) (w, h int, err error)` (default decodes the file header) so tests can inject sizes.

- [ ] **Step 1: Write the failing test**

```go
func TestRenderImageElementFallbackChip(t *testing.T) {
	reg := newImageRegistry()
	lines, h := renderImageElement(reg, false, "/note", "assets/pic.png", 40)
	if h != 1 || len(lines) != 1 {
		t.Fatalf("fallback should be single row, got h=%d lines=%d", h, len(lines))
	}
	if !strings.Contains(lines[0], "pic.png") || !strings.Contains(lines[0], "🖼") {
		t.Errorf("chip content wrong: %q", lines[0])
	}
}

func TestRenderImageElementKitty(t *testing.T) {
	old := imageDimensions
	defer func() { imageDimensions = old }()
	imageDimensions = func(path string) (int, int, error) { return 160, 320, nil } // 20x20 cells @ default
	reg := newImageRegistry()
	lines, h := renderImageElement(reg, true, "/note", "assets/pic.png", 40)
	if h < 1 || len(lines) != h {
		t.Fatalf("kitty rows mismatch: h=%d lines=%d", h, len(lines))
	}
	if !strings.Contains(lines[0], "\x1b_G") {
		t.Errorf("first render should include transmit sequence: %q", lines[0])
	}
	if strings.Count(lines[len(lines)-1], string(kitty.Placeholder)) == 0 {
		t.Errorf("rows should contain placeholder cells")
	}
	// Second render of the same asset must NOT re-transmit.
	lines2, _ := renderImageElement(reg, true, "/note", "assets/pic.png", 40)
	if strings.Contains(lines2[0], "\x1b_G") {
		t.Errorf("second render should not re-transmit: %q", lines2[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestRenderImageElement' -v`
Expected: FAIL — undefined `renderImageElement`, `imageDimensions`.

- [ ] **Step 3: Write minimal implementation**

Append to `imagerender.go` (add imports `image`, `os`, `path/filepath`; and blank imports for decoders `_ "image/png"`, `_ "image/jpeg"`, `_ "image/gif"`):

```go
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

func imageChip(target string) string {
	return "🖼 image (" + filepath.Base(target) + ")"
}

func renderImageElement(reg *imageRegistry, kittyOK bool, noteDir, target string, maxCols int) ([]string, int) {
	abs := resolveAssetPath(noteDir, target)
	data, err := os.ReadFile(abs)
	if !kittyOK || err != nil {
		return []string{imageChip(target)}, 1
	}
	w, h, derr := imageDimensions(abs)
	if derr != nil {
		return []string{imageChip(target)}, 1
	}
	if maxCols > maxImageCols {
		maxCols = maxImageCols
	}
	cols, rows := imageRenderSize(w, h, defaultCellW, defaultCellH, maxCols, maxImageRows)
	hash := assetFilename(data, "") // stable per content; date irrelevant for keying
	id := reg.idFor(hash)
	block := buildPlaceholderBlock(id, cols, rows)
	if reg.markTransmitted(hash) {
		block[0] = buildTransmitSequence(id, cols, rows, data) + block[0]
	}
	return block, rows
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestRenderImageElement' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add imagerender.go imagerender_test.go
git commit -m "feat(images): render image element to placeholder rows with fallback chip"
```

---

### Task 10: Splice image rendering into the editor

**Files:**
- Modify: `selection.go` (struct fields near line 18-29; `render()` at line 620-703)
- Modify: `model.go` — set editor fields when a note loads and on layout
- Test: `selection_test.go`

**Interfaces:**
- Consumes: `renderImageElement`, `parseImageElement`.
- Produces: `SelectableEditor` gains fields `imgReg *imageRegistry`, `kittyImages bool`, `noteDir string`; helper `func (e *SelectableEditor) SetImageContext(reg *imageRegistry, kitty bool, noteDir string)`.

**Design note:** In `render()`, the loop iterates soft-wrapped `rows`. For an image-element logical line, we must NOT emit the raw link text or wrap it. Instead, when the row is the *first* visual row of a logical line and that line parses as an image element, emit the image block (its own rows) and skip the remaining wrap rows for that logical line. Because image links are short they occupy a single wrap row anyway, so skipping is simple: detect on `r.startCol == 0`.

- [ ] **Step 1: Write the failing test**

```go
func TestEditorRendersImageChip(t *testing.T) {
	e := newSelectableEditor()
	e.SetSize(40, 10) // whatever the existing sizing method is; see setEditorSize
	e.SetImageContext(newImageRegistry(), false, t.TempDir())
	e.SetValue("before\n![](assets/pic.png)\nafter")
	out := e.render()
	if !strings.Contains(out, "🖼 image (pic.png)") {
		t.Errorf("editor did not render image chip:\n%s", out)
	}
	if strings.Contains(out, "assets/pic.png)") {
		t.Errorf("editor leaked raw link text:\n%s", out)
	}
}
```

Adjust `e.SetSize(...)` to the editor's real sizing call (`setEditorSize` in `editor.go:34`); if it needs a `model`, construct via the existing test helpers already used in `selection_test.go` (mirror an existing test's setup).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestEditorRendersImageChip -v`
Expected: FAIL — `SetImageContext` undefined / raw link leaks.

- [ ] **Step 3: Write minimal implementation**

Add fields to `SelectableEditor` (selection.go:18):

```go
	imgReg      *imageRegistry
	kittyImages bool
	noteDir     string
```

Add the setter:

```go
func (e *SelectableEditor) SetImageContext(reg *imageRegistry, kitty bool, noteDir string) {
	e.imgReg = reg
	e.kittyImages = kitty
	e.noteDir = noteDir
}
```

In `render()` (selection.go:653, top of the `for i := e.visualYOffset; ...` loop), handle image lines. Right after computing `r := rows[i]`, insert:

```go
		if r.startCol == 0 && r.line < len(lines) {
			if target, ok := parseImageElement(lines[r.line]); ok && e.imgReg != nil {
				imgLines, _ := renderImageElement(e.imgReg, e.kittyImages, e.noteDir, target, wrapWidth)
				pad := strings.Repeat(" ", numWidth)
				for _, il := range imgLines {
					b.WriteString(lineNumberStyle.Render(pad))
					b.WriteString(" ")
					b.WriteString(il)
					b.WriteString("\n")
					drawnRows++
				}
				// Skip any remaining wrap rows belonging to this logical line.
				for i+1 < endIdx && rows[i+1].line == r.line {
					i++
				}
				continue
			}
		}
```

(Selection/cursor highlighting for the element is handled by Task 11's navigation making the element atomic; a fully-selected element still shows its pixels/chip — acceptable for this iteration.)

In `model.go`, wire the editor image context. Add a model field `imgReg *imageRegistry` (initialize in `newModel`: `imgReg: newImageRegistry(),`). After a note is loaded/opened (search for where `m.editor.SetValue(...)` is called on file open, and in `recalcLayout`/`setEditorSize` paths), call:

```go
	m.editor.SetImageContext(m.imgReg, m.kittyImages, m.currentNoteDir())
```

Add the helper to `model.go`:

```go
func (m *model) currentNoteDir() string {
	if m.currentFile != "" {
		return filepath.Dir(m.currentFile)
	}
	if m.newNoteDir != "" {
		return m.newNoteDir
	}
	return m.vault
}
```

(`filepath` is already imported in model.go.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestEditorRendersImageChip -v`
Expected: PASS.

- [ ] **Step 5: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add selection.go model.go selection_test.go
git commit -m "feat(images): render image elements inline in the editor"
```

---

### Task 11: Atomic cursor navigation over image elements

**Files:**
- Modify: `selection.go` (add pure helper + use it in horizontal move methods)
- Test: `selection_test.go`

**Interfaces:**
- Produces: `func atomicCol(line string, col int, movingRight bool) int` — if `line` is an image element, snap `col` to `0` (when moving left / at start) or `len([]rune(line))` (when moving right / past start); otherwise return `col` unchanged.
- Consumed by: `moveCursorLeft`/`moveCursorRight` in `selection.go`.

**Design note:** Because an image element is its own short line, vertical movement already treats it as one logical line. The only gap is horizontal movement landing "inside" the link text. `atomicCol` snaps the column to an edge so the cursor never rests within the link.

- [ ] **Step 1: Write the failing test**

```go
func TestAtomicCol(t *testing.T) {
	line := "![](assets/pic.png)"
	n := len([]rune(line))
	if atomicCol(line, 5, true) != n {
		t.Errorf("moving right into element should snap to end")
	}
	if atomicCol(line, 5, false) != 0 {
		t.Errorf("moving left into element should snap to start")
	}
	if atomicCol("plain text", 3, true) != 3 {
		t.Errorf("non-element column should be unchanged")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestAtomicCol -v`
Expected: FAIL — `undefined: atomicCol`.

- [ ] **Step 3: Write minimal implementation**

Add to `selection.go`:

```go
func atomicCol(line string, col int, movingRight bool) int {
	if _, ok := parseImageElement(line); !ok {
		return col
	}
	if movingRight {
		return len([]rune(line))
	}
	return 0
}
```

In `moveCursorRight` (find the method in selection.go), after it computes the new `line, col`, snap: if the cursor stayed on the same logical line and that line is an element, set `col = atomicCol(lineText, col, true)`. Concretely, wrap the existing horizontal-move body so that after moving, you apply:

```go
	lines := strings.Split(e.Value(), "\n")
	if e.Line() < len(lines) {
		newCol := atomicCol(lines[e.Line()], e.cursorCol(), true)  // true in moveCursorRight, false in moveCursorLeft
		if newCol != e.cursorCol() {
			e.moveTo(e.Line(), newCol)
		}
	}
```

Apply the mirror (with `false`) in `moveCursorLeft`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestAtomicCol -v`
Expected: PASS.

- [ ] **Step 5: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add selection.go selection_test.go
git commit -m "feat(images): atomic horizontal cursor navigation over image elements"
```

---

### Task 12: Paste dispatch — image or text on Ctrl+V

**Files:**
- Modify: `model.go` (the `case "ctrl+v":` block at line 701-712)
- Test: `image_paste_test.go` (new)

**Interfaces:**
- Consumes: `currentClipEnv`, `clipboardHasImage`, `readClipboardImage`, `imageToolAvailable`, `saveAsset`, `assetRelPath`, `currentNoteDir`, `runCapture` (injectable).
- Produces: `func (m *model) pasteImageOrText() tea.Cmd` — if the clipboard holds an image, save it and insert a link line, else fall back to `m.editor.Paste()`.

**Design note:** Insert the link as its own line. If the cursor is mid-line, insert a newline first so the element is alone on its line. Use the editor's undo-wrapped insert (`InsertString` inside `recordOp`/`commitOp` already used by `Paste`). Simplest: `m.editor.InsertImageLink(rel)` which inserts `"\n"+link+"\n"`-style content atomically; implement it on the editor.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"strings"
	"testing"
)

func TestPasteImageInsertsLinkAndSavesAsset(t *testing.T) {
	vault := t.TempDir()
	oldCap := runCapture
	defer func() { runCapture = oldCap }()
	// Probe says image present; read returns bytes. Route by args.
	runCapture = func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "TARGETS" || a == "--list-types" {
				return []byte("image/png\n"), nil
			}
		}
		return []byte("PNGBYTES"), nil
	}
	oldEnv := clipEnvForPaste
	defer func() { clipEnvForPaste = oldEnv }()
	clipEnvForPaste = func() clipEnv { return clipX11 }

	m := newModel(vault, nil, "", "")
	m.kittyImages = true
	m.currentFile = vault + "/note.md"
	m.editor.SetValue("")
	m.editor.SetImageContext(m.imgReg, true, m.currentNoteDir())

	_ = m.pasteImageOrText()

	if !strings.Contains(m.editor.Value(), "![](assets/img-") {
		t.Errorf("paste did not insert image link: %q", m.editor.Value())
	}
	// Asset file exists.
	if len(listAssets(t, vault)) != 1 {
		t.Errorf("expected one asset file written")
	}
}
```

Add a small `listAssets` test helper (globs `<vault>/assets/*.png`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestPasteImageInsertsLinkAndSavesAsset -v`
Expected: FAIL — `pasteImageOrText`/`clipEnvForPaste`/`InsertImageLink` undefined.

- [ ] **Step 3: Write minimal implementation**

Add an indirection for the environment (so tests inject it) in `imageclip.go`:

```go
var clipEnvForPaste = currentClipEnv
```

Add to `selection.go` an insert helper:

```go
func (e *SelectableEditor) InsertImageLink(link string) {
	pre := e.recordOp()
	// Ensure the element sits on its own line.
	line := e.Value()
	_ = line
	e.InsertString("\n" + link + "\n")
	e.commitOp(pre)
}
```

Add to `model.go`:

```go
func (m *model) pasteImageOrText() tea.Cmd {
	env := clipEnvForPaste()
	if env != clipNone && clipboardHasImage(env) {
		if !imageToolAvailable(env) {
			m.errMsg = "install wl-clipboard or xclip to paste images"
			return nil
		}
		data, err := readClipboardImage(env)
		if err != nil || len(data) == 0 {
			m.errMsg = "could not read clipboard image"
			return nil
		}
		date := time.Now().Format("2006-01-02")
		abs, err := saveAsset(m.vault, data, date)
		if err != nil {
			m.errMsg = "could not save image: " + err.Error()
			return nil
		}
		rel := assetRelPath(m.currentNoteDir(), abs)
		m.editor.InsertImageLink("![](" + rel + ")")
		return nil
	}
	return m.editor.Paste()
}
```

(Add `time` to model.go imports if missing.)

Replace the editor branch of `case "ctrl+v":` (model.go:708-711) with:

```go
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				cmd := m.pasteImageOrText()
				return m, cmd
			}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestPasteImageInsertsLinkAndSavesAsset -v`
Expected: PASS.

- [ ] **Step 5: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add imageclip.go selection.go model.go image_paste_test.go
git commit -m "feat(images): paste clipboard image as an inline element on Ctrl+V"
```

---

### Task 13: Copy/Cut a lone image element back to the clipboard

**Files:**
- Modify: `model.go` (the `case "ctrl+c":`/`case "ctrl+x":` editor branches at lines 681-699)
- Modify: `selection.go` (add a predicate + helpers)
- Test: `image_copy_test.go` (new)

**Interfaces:**
- Produces:
  - `func (e *SelectableEditor) LoneImageElement() (target string, ok bool)` — true when there is no active multi-cell text selection and the cursor's current line is an image element (or the active selection is exactly that one line).
  - `func (m *model) copyImageElement(target string, cut bool) bool` — reads the asset bytes, writes them to the system clipboard as an image; on cut, deletes the element line. Returns false if it could not handle it (caller falls back to text copy/cut).
- Consumes: `resolveAssetPath`, `writeClipboardImage`, `imageToolAvailable`, `clipEnvForPaste`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"
)

func TestCopyLoneImageWritesBytes(t *testing.T) {
	vault := t.TempDir()
	// Create an asset the link points to.
	abs, _ := saveAsset(vault, []byte("REALPNG"), "2026-07-03")
	rel := assetRelPath(vault, abs)

	var wrote []byte
	oldW := runWithStdin
	defer func() { runWithStdin = oldW }()
	runWithStdin = func(stdin []byte, name string, args ...string) error { wrote = stdin; return nil }
	oldLook := imageToolAvailable
	_ = oldLook // imageToolAvailable is a func, not var; see note below

	oldEnv := clipEnvForPaste
	defer func() { clipEnvForPaste = oldEnv }()
	clipEnvForPaste = func() clipEnv { return clipWayland }

	m := newModel(vault, nil, "", "")
	m.currentFile = vault + "/note.md"
	m.editor.SetValue("![](" + rel + ")")
	m.editor.moveTo(0, 0)

	target, ok := m.editor.LoneImageElement()
	if !ok {
		t.Fatal("cursor on image line should report lone element")
	}
	if !m.copyImageElement(target, false) {
		t.Fatal("copyImageElement should succeed")
	}
	if string(wrote) != "REALPNG" {
		t.Errorf("wrote %q to clipboard, want REALPNG", wrote)
	}
}
```

Note: `imageToolAvailable` is a function in Task 4. To make copy testable without the real tool, add a package var seam `var imageToolAvailableFn = imageToolAvailable` and call the var from `copyImageElement`; in the test, override `imageToolAvailableFn` to return true. Adjust the test to set `imageToolAvailableFn = func(clipEnv) bool { return true }` and restore it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestCopyLoneImageWritesBytes -v`
Expected: FAIL — undefined `LoneImageElement`/`copyImageElement`.

- [ ] **Step 3: Write minimal implementation**

In `imageclip.go` add the seam:

```go
var imageToolAvailableFn = imageToolAvailable
```

In `selection.go`:

```go
func (e *SelectableEditor) LoneImageElement() (string, bool) {
	lines := strings.Split(e.Value(), "\n")
	cur := e.Line()
	if cur < 0 || cur >= len(lines) {
		return "", false
	}
	// A lone element: either no active selection, or the selection covers
	// exactly this one line.
	if e.selActive {
		sL, _, eL, _ := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
		if sL != eL {
			return "", false
		}
	}
	return parseImageElement(lines[cur])
}

func (e *SelectableEditor) DeleteCurrentLine() {
	pre := e.recordOp()
	lines := strings.Split(e.Value(), "\n")
	cur := e.Line()
	if cur < 0 || cur >= len(lines) {
		return
	}
	lines = append(lines[:cur], lines[cur+1:]...)
	if len(lines) == 0 {
		lines = []string{""}
	}
	e.SetValue(strings.Join(lines, "\n"))
	target := cur
	if target >= len(lines) {
		target = len(lines) - 1
	}
	e.moveTo(target, 0)
	e.selActive = false
	e.commitOp(pre)
}
```

In `model.go`:

```go
func (m *model) copyImageElement(target string, cut bool) bool {
	env := clipEnvForPaste()
	if env == clipNone || !imageToolAvailableFn(env) {
		return false
	}
	abs := resolveAssetPath(m.currentNoteDir(), target)
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	if err := writeClipboardImage(env, data); err != nil {
		return false
	}
	if cut {
		m.editor.DeleteCurrentLine()
	}
	m.errMsg = "Copied image to clipboard"
	return true
}
```

Update the editor branches of Ctrl+C and Ctrl+X in `model.go`:

```go
			// ctrl+c editor branch:
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				if target, ok := m.editor.LoneImageElement(); ok {
					if m.copyImageElement(target, false) {
						return m, nil
					}
				}
				m.editor.Copy()
			}
			return m, nil
```

```go
			// ctrl+x editor branch:
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				if target, ok := m.editor.LoneImageElement(); ok {
					if m.copyImageElement(target, true) {
						return m, nil
					}
				}
				m.editor.Cut()
			}
			return m, nil
```

(`os` is already imported in model.go.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestCopyLoneImageWritesBytes -v`
Expected: PASS.

- [ ] **Step 5: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add imageclip.go selection.go model.go image_copy_test.go
git commit -m "feat(images): copy/cut a lone image element writes bytes to clipboard"
```

---

### Task 14: Render images in the glamour preview

**Files:**
- Modify: `preview.go`
- Test: `preview_test.go` (new)

**Interfaces:**
- Consumes: `parseImageElement`, `renderImageElement`, `imageRegistry`.
- Produces: `func injectPreviewImages(rendered string, reg *imageRegistry, kitty bool, noteDir string) string` — replaces sentinel lines with placeholder blocks. And a pre-processing step `func stripImagesForGlamour(md string) (clean string, elements []string)` that replaces each image-element line with a unique sentinel `\x00IMG<i>\x00` before glamour renders, so glamour never emits alt-text.

**Design note:** glamour reflows and styles text, so post-matching raw `![](…)` is unreliable. Instead: (1) before rendering, replace each image-element line with a sentinel token on its own line; (2) render with glamour; (3) replace each sentinel line in the output with the image's placeholder rows (or chip). Wire this into `newPreviewViewport`, which needs the registry/kitty/noteDir — thread them through as parameters and update the one call site in `model.go`.

- [ ] **Step 1: Write the failing test**

```go
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
	out := injectPreviewImages(clean, newImageRegistry(), false, "/note")
	if !strings.Contains(out, "🖼 image (pic.png)") {
		t.Errorf("chip not injected: %q", out)
	}
	if strings.Contains(out, "\x00IMG0\x00") {
		t.Errorf("sentinel not replaced: %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestStripAndInjectPreviewImages -v`
Expected: FAIL — undefined `stripImagesForGlamour`/`injectPreviewImages`.

- [ ] **Step 3: Write minimal implementation**

Add to `preview.go` (add imports `fmt`, `strings`):

```go
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

func injectPreviewImages(rendered string, reg *imageRegistry, kitty bool, noteDir string) string {
	lines := strings.Split(rendered, "\n")
	for i, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		var idx int
		if n, err := fmt.Sscanf(trimmed, "\x00IMG%d\x00", &idx); err == nil && n == 1 {
			// Look up which target this sentinel referred to via the registry-independent map.
		}
		_ = i
	}
	// Simpler: replace by scanning for each sentinel with its known target.
	return rendered
}
```

The `injectPreviewImages` above is a stub — replace it with a version that takes the `els` slice so it knows each sentinel's target:

```go
func injectPreviewImages(rendered string, els []string, reg *imageRegistry, kitty bool, noteDir string) string {
	for i, target := range els {
		sentinel := fmt.Sprintf("\x00IMG%d\x00", i)
		block, _ := renderImageElement(reg, kitty, noteDir, target, maxImageCols)
		rendered = strings.Replace(rendered, sentinel, strings.Join(block, "\n"), 1)
	}
	return rendered
}
```

Update the test's `injectPreviewImages` call to pass `els`:
`injectPreviewImages(clean, els, newImageRegistry(), false, "/note")` and change the test's earlier `clean, els := ...` accordingly (already captured). Update the test call site to include `els`.

Thread through `newPreviewViewport`:

```go
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
```

Update the single call site in `model.go` (search `newPreviewViewport(`) to pass `m.imgReg, m.kittyImages, m.currentNoteDir()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestStripAndInjectPreviewImages -v`
Expected: PASS. (Update the test's `injectPreviewImages` argument list to include `els` as noted.)

- [ ] **Step 5: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add preview.go model.go preview_test.go
git commit -m "feat(images): render inline images in the glamour preview"
```

---

### Task 15: Integration wiring, full build, and manual verification

**Files:**
- Modify: `main.go` (optional: nothing more needed if detection is env-based in `newModel`), any missed call sites.
- Verify only; no new test file.

- [ ] **Step 1: Full build and vet**

Run: `go build ./... && go vet ./...`
Expected: no errors. Fix any missed imports or call sites (e.g. remaining `newPreviewViewport` callers, editor `SetImageContext` on note open).

- [ ] **Step 2: Full test suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 3: Confirm no regression in existing clipboard/selection tests**

Run: `go test ./... -run 'Selection|Clip|Undo|Mouse' -v`
Expected: PASS.

- [ ] **Step 4: Manual verification checklist (real kitty-protocol terminal, e.g. kitty/ghostty)**

Build and run: `go build -o clipad . && ./clipad`

1. Copy a screenshot to the clipboard; place the cursor on an empty line; Ctrl+V → the image renders inline as pixels; an `assets/img-*.png` file appears; the `.md` gains a `![](assets/…)` link.
2. Scroll past the image and back → it redraws with no artifacts.
3. Resize the terminal → the image reflows/rescales without corruption.
4. Move the cursor onto the element and press Delete/Backspace → the element is removed as one step; Ctrl+Z restores it.
5. Put the cursor on the element, Ctrl+C, then paste into an external image app → the real image appears.
6. Ctrl+C on the element, then Ctrl+V into another note → the image re-renders (idempotent asset reuse — same filename).
7. Ctrl+P (preview) → the image renders as pixels there too.
8. Run in a non-kitty terminal (e.g. `TERM=xterm-256color ./clipad`) → the element shows the `🖼 image (name)` chip; cut/copy/delete still work.
9. Temporarily rename `wl-copy`/`xclip` off PATH and paste an image → status line shows `install wl-clipboard or xclip to paste images`; text paste still works.

- [ ] **Step 5: Commit any fixes found during manual verification**

```bash
git add -A
git commit -m "fix(images): address issues found in manual verification"
```

---

## Self-Review

**Spec coverage:**
- Paste image inline (editor pixels) → Tasks 9, 10, 12. ✓
- Pixels in glamour preview → Task 14. ✓
- Atomic element (nav/delete/cut/copy) → Tasks 10, 11, 13. ✓
- Copy/cut lone element writes image bytes to clipboard → Task 13. ✓
- File-backed, content-addressed `assets/`, relative links → Task 2, used in 12. ✓
- Smart Ctrl+V (image-or-text) → Task 12. ✓
- Wayland/X11 auto-detect + shell out, no new Go deps → Tasks 3, 4. ✓
- Kitty capability detection + text-chip fallback everywhere → Tasks 5, 9 (fallback), 10, 14. ✓
- Mixed-selection copy stays text → Task 13 (`LoneImageElement` returns false for multi-line selections; editor branches fall through to `Copy()`/`Cut()`). ✓
- Semantic index unaffected (links are short) → no task needed; no base64 enters notes. ✓
- Non-goals (sixel, drag-drop, orphan GC) → intentionally absent. ✓

**Placeholder scan:** Task 7 contains an explicitly-labeled *draft-then-final* for `buildTransmitSequence`; the step instructs deleting the draft and keeping the clean version + `chunkString`. Task 14 Step 3 shows a stub `injectPreviewImages` then its real signature; the step instructs replacing the stub and updating the call/test to pass `els`. These are guided rewrites, not unfilled placeholders. No `TODO`/`TBD` remain.

**Type consistency:**
- `imageRegistry` methods `idFor`/`markTransmitted` — consistent across Tasks 8, 9.
- `renderImageElement(reg, kitty, noteDir, target, maxCols)` — same signature in Tasks 9, 10, 14.
- `clipEnv` values `clipNone/clipWayland/clipX11` — consistent Tasks 3, 4, 12, 13.
- `clipEnvForPaste`, `imageToolAvailableFn` seams — introduced in Tasks 12/13 and used consistently.
- `SetImageContext(reg, kitty, noteDir)` — Tasks 10, and preview threads the same trio.
- `currentNoteDir()` — defined Task 10, used Tasks 12, 13, 14.
