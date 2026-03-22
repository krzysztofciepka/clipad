# Clipad Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a TUI note-taking app with an Obsidian-like layout (file tree + markdown editor + preview) in Go.

**Architecture:** Bubble Tea Elm architecture with a single model. File tree on the left, editor/preview on the right, status bar at the bottom. Config stored in TOML, markdown rendered via Glamour.

**Tech Stack:** Go, Bubble Tea, Bubbles (textarea/viewport/textinput), Lipgloss, Glamour, go-toml/v2, sahilm/fuzzy

**Spec:** `docs/superpowers/specs/2026-03-22-clipad-design.md`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `main.go` | Entry point: load config, run first-run setup or main app |
| `config.go` | Config struct, TOML read/write, XDG path resolution, vault validation |
| `config_test.go` | Tests for config loading, saving, XDG resolution |
| `filetree_item.go` | `TreeNode` struct (name, path, isDir, children, expanded), tree building from filesystem, sorting, flattening for display |
| `filetree_item_test.go` | Tests for tree building, sorting, flattening |
| `tree.go` | Tree panel Bubble Tea component: navigation, expand/collapse, selection, rendering |
| `filter.go` | Fuzzy filter logic: collect all file paths from tree, fuzzy match, return results |
| `filter_test.go` | Tests for fuzzy filtering |
| `editor.go` | Editor wrapper: textarea setup, resize, content get/set, dirty tracking |
| `preview.go` | Preview component: Glamour rendering into viewport, scroll handling |
| `statusbar.go` | Bottom bar rendering: keybindings, cursor position, filename, modified indicator, error messages |
| `model.go` | Main model: all state, Init/Update/View, keybinding dispatch, panel layout, modal prompts (new note, delete confirm, unsaved guard) |

---

### Task 1: Project Scaffolding & Config

**Files:**
- Create: `go.mod`
- Create: `config.go`
- Create: `config_test.go`
- Create: `main.go`

- [ ] **Step 1: Initialize Go module and install dependencies**

```bash
cd /home/kc/repos/clipad
go mod init clipad
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/glamour@latest
go get github.com/pelletier/go-toml/v2@latest
go get github.com/sahilm/fuzzy@latest
```

- [ ] **Step 2: Write failing tests for config**

Create `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPath_XDGSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := configPath()
	want := "/tmp/test-xdg/clipad/config.toml"
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestConfigPath_XDGUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	got := configPath()
	want := filepath.Join(home, ".config", "clipad", "config.toml")
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg := Config{Vault: "/tmp/my-vault"}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if loaded.Vault != cfg.Vault {
		t.Errorf("loaded.Vault = %q, want %q", loaded.Vault, cfg.Vault)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test -v -run TestConfig
```
Expected: FAIL — `configPath`, `loadConfig`, `saveConfig`, `Config` not defined.

- [ ] **Step 4: Implement config.go**

Create `config.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Vault string `toml:"vault"`
}

func configPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "config.toml")
}

func loadConfig() (Config, error) {
	var cfg Config
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -v -run TestConfig
```
Expected: PASS — all 4 tests green.

- [ ] **Step 6: Create minimal main.go**

Create `main.go` — just enough to compile and be extended later:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "No config found. Run setup first.\n")
		os.Exit(1)
	}
	fmt.Printf("Vault: %s\n", cfg.Vault)
}
```

- [ ] **Step 7: Verify build compiles**

```bash
go build -o clipad .
```
Expected: compiles without errors, produces `clipad` binary.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum config.go config_test.go main.go
git commit -m "feat: add config loading with XDG support and main entry point"
```

---

### Task 2: File Tree Data Structure

**Files:**
- Create: `filetree_item.go`
- Create: `filetree_item_test.go`

- [ ] **Step 1: Write failing tests for tree building**

Create `filetree_item_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create structure:
	//   notes/
	//     daily/
	//       jan.md
	//     ideas.md
	//   projects/
	//     todo.md
	//   readme.md
	//   .hidden/
	//     secret.md
	os.MkdirAll(filepath.Join(dir, "notes", "daily"), 0o755)
	os.MkdirAll(filepath.Join(dir, "projects"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, "notes", "daily", "jan.md"), []byte("# Jan"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "ideas.md"), []byte("# Ideas"), 0o644)
	os.WriteFile(filepath.Join(dir, "projects", "todo.md"), []byte("# Todo"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Readme"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("# Secret"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "ignore.txt"), []byte("not markdown"), 0o644)
	return dir
}

func TestBuildTree(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	// Root should have children: notes/, projects/, readme.md
	// .hidden/ should be excluded, ignore.txt should be excluded
	if len(root.Children) != 3 {
		t.Errorf("root has %d children, want 3", len(root.Children))
	}
}

func TestBuildTree_SortOrder(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	// Folders first (notes, projects), then files (readme.md)
	if root.Children[0].Name != "notes" {
		t.Errorf("first child = %q, want %q", root.Children[0].Name, "notes")
	}
	if root.Children[1].Name != "projects" {
		t.Errorf("second child = %q, want %q", root.Children[1].Name, "projects")
	}
	if root.Children[2].Name != "readme.md" {
		t.Errorf("third child = %q, want %q", root.Children[2].Name, "readme.md")
	}
}

func TestBuildTree_HiddenExcluded(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	for _, child := range root.Children {
		if child.Name == ".hidden" {
			t.Error("hidden directory should be excluded")
		}
	}
}

func TestFlattenTree(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	// Expand all folders
	expandAll(root)
	items := flattenTree(root, 0)

	// Should have: notes(0), daily(1), jan.md(2), ideas.md(1), projects(0), todo.md(1), readme.md(0)
	if len(items) != 7 {
		names := make([]string, len(items))
		for i, item := range items {
			names[i] = item.Node.Name
		}
		t.Errorf("flattenTree() returned %d items %v, want 7", len(items), names)
	}
}

func TestCollectFiles(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	files := collectFiles(root)
	// Should have 3 .md files: jan.md, ideas.md, todo.md, readme.md
	if len(files) != 4 {
		t.Errorf("collectFiles() returned %d files, want 4", len(files))
	}
}

func expandAll(node *TreeNode) {
	if node.IsDir {
		node.Expanded = true
		for _, child := range node.Children {
			expandAll(child)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -v -run TestBuildTree -run TestFlattenTree -run TestCollectFiles
```
Expected: FAIL — `TreeNode`, `buildTree`, `flattenTree`, `collectFiles` not defined.

- [ ] **Step 3: Implement filetree_item.go**

Create `filetree_item.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type TreeNode struct {
	Name     string
	Path     string
	IsDir    bool
	Expanded bool
	Children []*TreeNode
}

type FlatItem struct {
	Node  *TreeNode
	Depth int
}

func buildTree(root string) (*TreeNode, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	node := &TreeNode{
		Name:  info.Name(),
		Path:  root,
		IsDir: true,
	}
	if err := populateChildren(node); err != nil {
		return nil, err
	}
	return node, nil
}

func populateChildren(node *TreeNode) error {
	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return err
	}

	var dirs, files []*TreeNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		childPath := filepath.Join(node.Path, name)
		if entry.IsDir() {
			child := &TreeNode{
				Name:  name,
				Path:  childPath,
				IsDir: true,
			}
			if err := populateChildren(child); err != nil {
				continue
			}
			// Only include dirs that have .md files (directly or nested)
			if hasMarkdownFiles(child) {
				dirs = append(dirs, child)
			}
		} else if strings.HasSuffix(strings.ToLower(name), ".md") {
			files = append(files, &TreeNode{
				Name: name,
				Path: childPath,
			})
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	node.Children = append(dirs, files...)
	return nil
}

func hasMarkdownFiles(node *TreeNode) bool {
	for _, child := range node.Children {
		if !child.IsDir {
			return true
		}
		if hasMarkdownFiles(child) {
			return true
		}
	}
	return false
}

func flattenTree(node *TreeNode, depth int) []FlatItem {
	var items []FlatItem
	for _, child := range node.Children {
		items = append(items, FlatItem{Node: child, Depth: depth})
		if child.IsDir && child.Expanded {
			items = append(items, flattenTree(child, depth+1)...)
		}
	}
	return items
}

func collectFiles(node *TreeNode) []*TreeNode {
	var files []*TreeNode
	for _, child := range node.Children {
		if child.IsDir {
			files = append(files, collectFiles(child)...)
		} else {
			files = append(files, child)
		}
	}
	return files
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -v -run "TestBuildTree|TestFlattenTree|TestCollectFiles"
```
Expected: PASS — all 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add filetree_item.go filetree_item_test.go
git commit -m "feat: add file tree data structure with build, flatten, and collect"
```

---

### Task 3: Fuzzy Filter Logic

**Files:**
- Create: `filter.go`
- Create: `filter_test.go`

- [ ] **Step 1: Write failing tests for filter**

Create `filter_test.go`:

```go
package main

import (
	"testing"
)

func TestFilterFiles_ExactMatch(t *testing.T) {
	files := []*TreeNode{
		{Name: "ideas.md", Path: "/vault/ideas.md"},
		{Name: "todo.md", Path: "/vault/todo.md"},
		{Name: "readme.md", Path: "/vault/readme.md"},
	}
	results := filterFiles(files, "todo")
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "todo.md" {
		t.Errorf("first result = %q, want %q", results[0].Name, "todo.md")
	}
}

func TestFilterFiles_FuzzyMatch(t *testing.T) {
	files := []*TreeNode{
		{Name: "meeting-notes.md", Path: "/vault/meeting-notes.md"},
		{Name: "readme.md", Path: "/vault/readme.md"},
	}
	results := filterFiles(files, "mtn")
	if len(results) == 0 {
		t.Fatal("expected fuzzy match for 'mtn' -> 'meeting-notes.md'")
	}
}

func TestFilterFiles_EmptyQuery(t *testing.T) {
	files := []*TreeNode{
		{Name: "a.md", Path: "/vault/a.md"},
		{Name: "b.md", Path: "/vault/b.md"},
	}
	results := filterFiles(files, "")
	if len(results) != 2 {
		t.Errorf("empty query should return all files, got %d", len(results))
	}
}

func TestFilterFiles_NoMatch(t *testing.T) {
	files := []*TreeNode{
		{Name: "ideas.md", Path: "/vault/ideas.md"},
	}
	results := filterFiles(files, "zzzzz")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -v -run TestFilterFiles
```
Expected: FAIL — `filterFiles` not defined.

- [ ] **Step 3: Implement filter.go**

Create `filter.go`:

```go
package main

import (
	"github.com/sahilm/fuzzy"
)

type fileSource []*TreeNode

func (f fileSource) String(i int) string {
	return f[i].Name
}

func (f fileSource) Len() int {
	return len(f)
}

func filterFiles(files []*TreeNode, query string) []*TreeNode {
	if query == "" {
		return files
	}
	src := fileSource(files)
	matches := fuzzy.FindFrom(query, src)
	results := make([]*TreeNode, len(matches))
	for i, m := range matches {
		results[i] = files[m.Index]
	}
	return results
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -v -run TestFilterFiles
```
Expected: PASS — all 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add filter.go filter_test.go
git commit -m "feat: add fuzzy file filtering with sahilm/fuzzy"
```

---

### Task 4: Status Bar Component

**Files:**
- Create: `statusbar.go`

- [ ] **Step 1: Implement statusbar.go**

Create `statusbar.go`:

```go
package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	statusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	statusErrorStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("196")).
		Bold(true)
)

type StatusBar struct {
	width      int
	treeActive bool
	filename   string
	line       int
	col        int
	dirty      bool
	errMsg     string
}

func (s StatusBar) View() string {
	left := statusKeyStyle.Render("^S") + " save  " +
		statusKeyStyle.Render("^N") + " new  "

	if s.treeActive {
		left += statusKeyStyle.Render("^D") + " del  "
	}

	left += statusKeyStyle.Render("^Q") + " quit  " +
		statusKeyStyle.Render("Tab") + " switch  " +
		statusKeyStyle.Render("^P") + " preview"

	right := ""
	if s.errMsg != "" {
		right = statusErrorStyle.Render(s.errMsg)
	} else if s.filename != "" {
		modified := ""
		if s.dirty {
			modified = " [+]"
		}
		right = fmt.Sprintf("%d:%d  %s%s", s.line, s.col, s.filename, modified)
	}

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	padding := ""
	for i := 0; i < gap; i++ {
		padding += " "
	}

	bar := left + padding + right
	return statusBarStyle.Width(s.width).Render(bar)
}
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add statusbar.go
git commit -m "feat: add status bar component with keybindings and file info"
```

---

### Task 5: Editor Wrapper

**Files:**
- Create: `editor.go`

- [ ] **Step 1: Implement editor.go**

Create `editor.go`:

```go
package main

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

var editorStyle = lipgloss.NewStyle().Padding(0, 1)

func newEditor() textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = true
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.CharLimit = 0 // no limit
	ta.Focus()
	return ta
}

func setEditorSize(ta *textarea.Model, width, height int) {
	ta.SetWidth(width - 2)  // account for padding
	ta.SetHeight(height)
}

func editorCursorPos(ta textarea.Model) (line, col int) {
	return ta.Line(), ta.CursorPosition()
}
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add editor.go
git commit -m "feat: add editor wrapper around bubbles textarea"
```

---

### Task 6: Preview Component

**Files:**
- Create: `preview.go`

- [ ] **Step 1: Implement preview.go**

Create `preview.go`:

```go
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
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add preview.go
git commit -m "feat: add preview component with Glamour markdown rendering"
```

---

### Task 7: Tree Panel Component

**Files:**
- Create: `tree.go`

- [ ] **Step 1: Implement tree.go**

Create `tree.go`:

```go
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	treePanelStyle = lipgloss.NewStyle().
		Padding(0, 1).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder())

	treeSelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	treeDirStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("75")).
		Bold(true)

	treeFileStyle  = lipgloss.NewStyle()
	treeActiveFile = lipgloss.NewStyle().
			Foreground(lipgloss.Color("156"))
)

type TreePanel struct {
	root        *TreeNode
	items       []FlatItem
	cursor      int
	offset      int
	height      int
	width       int
	currentFile string
}

func newTreePanel(root *TreeNode, width, height int) TreePanel {
	tp := TreePanel{
		root:   root,
		width:  width,
		height: height,
	}
	tp.rebuildItems()
	return tp
}

func (tp *TreePanel) rebuildItems() {
	if tp.root != nil {
		tp.items = flattenTree(tp.root, 0)
	} else {
		tp.items = nil
	}
}

func (tp *TreePanel) moveUp() {
	if tp.cursor > 0 {
		tp.cursor--
		if tp.cursor < tp.offset {
			tp.offset = tp.cursor
		}
	}
}

func (tp *TreePanel) moveDown() {
	if tp.cursor < len(tp.items)-1 {
		tp.cursor++
		if tp.cursor >= tp.offset+tp.height {
			tp.offset = tp.cursor - tp.height + 1
		}
	}
}

func (tp *TreePanel) toggleOrSelect() *TreeNode {
	if tp.cursor >= len(tp.items) {
		return nil
	}
	item := tp.items[tp.cursor]
	if item.Node.IsDir {
		item.Node.Expanded = !item.Node.Expanded
		tp.rebuildItems()
		// Clamp cursor
		if tp.cursor >= len(tp.items) {
			tp.cursor = len(tp.items) - 1
		}
		return nil
	}
	return item.Node
}

func (tp *TreePanel) selectedNode() *TreeNode {
	if tp.cursor >= 0 && tp.cursor < len(tp.items) {
		return tp.items[tp.cursor].Node
	}
	return nil
}

func (tp TreePanel) View(focused bool) string {
	var b strings.Builder

	end := tp.offset + tp.height
	if end > len(tp.items) {
		end = len(tp.items)
	}

	for i := tp.offset; i < end; i++ {
		item := tp.items[i]
		indent := strings.Repeat("  ", item.Depth)

		var icon, name string
		if item.Node.IsDir {
			if item.Node.Expanded {
				icon = "▼ "
			} else {
				icon = "▶ "
			}
			name = treeDirStyle.Render(item.Node.Name)
		} else {
			icon = "  "
			if item.Node.Path == tp.currentFile {
				name = treeActiveFile.Render(item.Node.Name)
			} else {
				name = treeFileStyle.Render(item.Node.Name)
			}
		}

		line := fmt.Sprintf("%s%s%s", indent, icon, name)

		if i == tp.cursor && focused {
			// Pad to full width for selection highlight
			padded := line
			lineWidth := lipgloss.Width(padded)
			if lineWidth < tp.width-2 {
				padded += strings.Repeat(" ", tp.width-2-lineWidth)
			}
			line = treeSelectedStyle.Render(padded)
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines
	rendered := b.String()
	lineCount := end - tp.offset
	for i := lineCount; i < tp.height; i++ {
		rendered += "\n"
	}

	return treePanelStyle.Width(tp.width).Height(tp.height).Render(rendered)
}
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add tree.go
git commit -m "feat: add tree panel component with navigation and rendering"
```

---

### Task 8: Main Model — Layout, Keybindings, and Core Loop

**Files:**
- Create: `model.go`
- Modify: `main.go`

This is the biggest task — it wires everything together.

- [ ] **Step 1: Implement model.go**

Create `model.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type panel int

const (
	treePanel panel = iota
	editorPanel
)

type editorMode int

const (
	modeEdit editorMode = iota
	modePreview
)

type pendingActionType int

const (
	pendingNone pendingActionType = iota
	pendingSwitchFile
	pendingQuit
)

type inputMode int

const (
	inputNone inputMode = iota
	inputNewNote
	inputFilter
	inputConfirmDelete
	inputUnsavedGuard
)

type model struct {
	// Layout
	width  int
	height int

	// Panels
	activePanel panel
	editorMode  editorMode

	// Vault
	vault string

	// Tree
	tree       TreePanel
	treeRoot   *TreeNode
	treeWidth  int
	treeHeight int

	// Editor
	editor       textarea.Model
	editorWidth  int
	editorHeight int

	// Preview
	preview viewport.Model

	// File state
	currentFile string
	dirty       bool

	// Input overlays
	inputMode     inputMode
	newNoteInput  textinput.Model
	filterInput   textinput.Model
	filterResults []*TreeNode
	filterCursor  int
	filterOffset  int

	// Pending action (unsaved changes guard)
	pendingAction     pendingActionType
	pendingSwitchPath string

	// Status
	errMsg string
}

func newModel(vault string) model {
	ni := textinput.New()
	ni.Placeholder = "path/to/note.md"
	ni.CharLimit = 256

	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.CharLimit = 256

	m := model{
		vault:       vault,
		activePanel: treePanel,
		editorMode:  modeEdit,
		editor:      newEditor(),
		newNoteInput: ni,
		filterInput:  fi,
	}

	root, err := buildTree(vault)
	if err != nil {
		m.errMsg = fmt.Sprintf("Error reading vault: %v", err)
	} else {
		m.treeRoot = root
	}

	return m
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case tea.KeyMsg:
		// Handle input overlays first
		if m.inputMode != inputNone {
			return m.handleInputMode(msg)
		}

		// Global keybindings
		switch msg.String() {
		case "ctrl+q":
			if m.dirty {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingQuit
				return m, nil
			}
			return m, tea.Quit

		case "ctrl+s":
			m.saveCurrentFile()
			return m, nil

		case "ctrl+n":
			m.inputMode = inputNewNote
			m.newNoteInput.SetValue("")
			m.newNoteInput.Focus()
			return m, textinput.Blink

		case "ctrl+p":
			return m.togglePreview()

		case "tab":
			if m.activePanel == treePanel {
				m.activePanel = editorPanel
				m.editor.Focus()
			} else {
				m.activePanel = treePanel
				m.editor.Blur()
			}
			return m, nil
		}

		// Panel-specific keybindings
		if m.activePanel == treePanel {
			return m.handleTreeKeys(msg)
		}
		return m.handleEditorKeys(msg)
	}

	// Pass messages to textarea when editing
	if m.activePanel == editorPanel && m.editorMode == modeEdit {
		var cmd tea.Cmd
		oldValue := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		if m.editor.Value() != oldValue {
			m.dirty = true
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) handleTreeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.tree.moveUp()
	case "down", "j":
		m.tree.moveDown()
	case "enter":
		node := m.tree.toggleOrSelect()
		if node != nil {
			if m.dirty {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingSwitchFile
				m.pendingSwitchPath = node.Path
				return m, nil
			}
			m.openFile(node.Path)
		}
	case "/":
		m.inputMode = inputFilter
		m.filterInput.SetValue("")
		m.filterInput.Focus()
		m.filterResults = collectFiles(m.treeRoot)
		m.filterCursor = 0
		m.filterOffset = 0
		return m, textinput.Blink
	case "ctrl+d":
		node := m.tree.selectedNode()
		if node != nil && !node.IsDir {
			m.inputMode = inputConfirmDelete
		}
	}
	return m, nil
}

func (m model) handleEditorKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editorMode == modePreview {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}

	oldValue := m.editor.Value()
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	if m.editor.Value() != oldValue {
		m.dirty = true
	}
	return m, cmd
}

func (m model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.inputMode {
	case inputNewNote:
		return m.handleNewNoteInput(msg)
	case inputFilter:
		return m.handleFilterInput(msg)
	case inputConfirmDelete:
		return m.handleDeleteConfirm(msg)
	case inputUnsavedGuard:
		return m.handleUnsavedGuard(msg)
	}
	return m, nil
}

func (m model) handleNewNoteInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		path := m.newNoteInput.Value()
		if path != "" {
			m.createAndOpenNote(path)
		}
		m.inputMode = inputNone
		return m, nil
	case "esc":
		m.inputMode = inputNone
		return m, nil
	}
	var cmd tea.Cmd
	m.newNoteInput, cmd = m.newNoteInput.Update(msg)
	return m, cmd
}

func (m model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.filterCursor < len(m.filterResults) {
			path := m.filterResults[m.filterCursor].Path
			m.inputMode = inputNone
			if m.dirty {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingSwitchFile
				m.pendingSwitchPath = path
				return m, nil
			}
			m.openFile(path)
		} else {
			m.inputMode = inputNone
		}
		return m, nil
	case "esc":
		m.inputMode = inputNone
		return m, nil
	case "up":
		if m.filterCursor > 0 {
			m.filterCursor--
			if m.filterCursor < m.filterOffset {
				m.filterOffset = m.filterCursor
			}
		}
		return m, nil
	case "down":
		if m.filterCursor < len(m.filterResults)-1 {
			m.filterCursor++
			maxVisible := m.treeHeight - 1
			if m.filterCursor >= m.filterOffset+maxVisible {
				m.filterOffset = m.filterCursor - maxVisible + 1
			}
		}
		return m, nil
	// Global keybindings remain active during filter
	case "ctrl+q":
		return m, tea.Quit
	case "ctrl+s":
		m.saveCurrentFile()
		return m, nil
	case "ctrl+p":
		m.inputMode = inputNone
		return m.togglePreview()
	case "ctrl+n":
		m.inputMode = inputNone
		m.newNoteInput.SetValue("")
		m.newNoteInput.Focus()
		m.inputMode = inputNewNote
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	// Update filter results
	allFiles := collectFiles(m.treeRoot)
	m.filterResults = filterFiles(allFiles, m.filterInput.Value())
	m.filterCursor = 0
	m.filterOffset = 0
	return m, cmd
}

func (m model) handleDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		node := m.tree.selectedNode()
		if node != nil {
			if err := os.Remove(node.Path); err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
			} else {
				if m.currentFile == node.Path {
					m.currentFile = ""
					m.editor.SetValue("")
					m.dirty = false
				}
				m.refreshTree()
			}
		}
		m.inputMode = inputNone
	case "n", "esc":
		m.inputMode = inputNone
	}
	return m, nil
}

func (m model) handleUnsavedGuard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.saveCurrentFile()
		return m.executePendingAction()
	case "n":
		m.dirty = false
		return m.executePendingAction()
	case "esc":
		m.inputMode = inputNone
		m.pendingAction = pendingNone
	}
	return m, nil
}

func (m model) executePendingAction() (tea.Model, tea.Cmd) {
	m.inputMode = inputNone
	switch m.pendingAction {
	case pendingQuit:
		m.pendingAction = pendingNone
		return m, tea.Quit
	case pendingSwitchFile:
		m.openFile(m.pendingSwitchPath)
		m.pendingAction = pendingNone
		m.pendingSwitchPath = ""
	}
	return m, nil
}

func (m *model) openFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		m.errMsg = fmt.Sprintf("Open failed: %v", err)
		return
	}
	m.currentFile = path
	m.editor.SetValue(string(data))
	m.dirty = false
	m.editorMode = modeEdit
	m.tree.currentFile = path
	m.errMsg = ""
}

func (m *model) saveCurrentFile() {
	if m.currentFile == "" {
		m.errMsg = "No file open"
		return
	}
	if err := os.WriteFile(m.currentFile, []byte(m.editor.Value()), 0o644); err != nil {
		m.errMsg = fmt.Sprintf("Save failed: %v", err)
		return
	}
	m.dirty = false
	m.errMsg = ""
	m.refreshTree()
}

func (m *model) createAndOpenNote(relPath string) {
	if !strings.HasSuffix(relPath, ".md") {
		relPath += ".md"
	}
	fullPath := filepath.Join(m.vault, relPath)

	// Check if exists — open instead
	if _, err := os.Stat(fullPath); err == nil {
		m.openFile(fullPath)
		return
	}

	// Create directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.errMsg = fmt.Sprintf("Create dir failed: %v", err)
		return
	}

	// Create empty file
	if err := os.WriteFile(fullPath, []byte(""), 0o644); err != nil {
		m.errMsg = fmt.Sprintf("Create file failed: %v", err)
		return
	}

	m.refreshTree()
	m.openFile(fullPath)
}

func (m *model) refreshTree() {
	root, err := buildTree(m.vault)
	if err != nil {
		m.errMsg = fmt.Sprintf("Refresh failed: %v", err)
		return
	}
	// Preserve expanded state
	if m.treeRoot != nil {
		copyExpandedState(m.treeRoot, root)
	}
	m.treeRoot = root
	m.tree.root = root
	m.tree.rebuildItems()
}

func copyExpandedState(old, new_ *TreeNode) {
	oldMap := make(map[string]bool)
	collectExpanded(old, oldMap)
	applyExpanded(new_, oldMap)
}

func collectExpanded(node *TreeNode, m map[string]bool) {
	if node.IsDir && node.Expanded {
		m[node.Path] = true
	}
	for _, child := range node.Children {
		collectExpanded(child, m)
	}
}

func applyExpanded(node *TreeNode, m map[string]bool) {
	if node.IsDir {
		node.Expanded = m[node.Path]
	}
	for _, child := range node.Children {
		applyExpanded(child, m)
	}
}

func (m model) togglePreview() (tea.Model, tea.Cmd) {
	if m.editorMode == modeEdit {
		vp, err := newPreviewViewport(m.editor.Value(), m.editorWidth, m.editorHeight)
		if err != nil {
			m.errMsg = fmt.Sprintf("Preview failed: %v", err)
			return m, nil
		}
		m.preview = vp
		m.editorMode = modePreview
	} else {
		m.editorMode = modeEdit
	}
	return m, nil
}

func (m *model) recalcLayout() {
	if m.width < 60 || m.height < 15 {
		return // too small, handled in View
	}

	m.treeWidth = m.width / 4
	m.editorWidth = m.width - m.treeWidth
	m.treeHeight = m.height - 2 // 1 for status bar, 1 for border
	m.editorHeight = m.height - 2

	m.tree.width = m.treeWidth
	m.tree.height = m.treeHeight

	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.width < 60 || m.height < 15 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			"Terminal too small\nMinimum: 60x15")
	}

	// Tree panel
	treeView := m.tree.View(m.activePanel == treePanel)

	// Filter overlay on tree
	if m.inputMode == inputFilter {
		treeView = m.filterView()
	}

	// Editor/Preview panel
	var rightView string
	if m.currentFile == "" {
		placeholder := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 2).
			Render("Select a file from the tree or press Ctrl+N to create a new note")
		rightView = lipgloss.NewStyle().
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(placeholder)
	} else if m.editorMode == modePreview {
		rightView = previewStyle.
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(m.preview.View())
	} else {
		rightView = editorStyle.
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(m.editor.View())
	}

	// Join panels
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, treeView, rightView)

	// Status bar
	line, col := editorCursorPos(m.editor)
	filename := ""
	if m.currentFile != "" {
		rel, err := filepath.Rel(m.vault, m.currentFile)
		if err != nil {
			filename = filepath.Base(m.currentFile)
		} else {
			filename = rel
		}
	}

	sb := StatusBar{
		width:      m.width,
		treeActive: m.activePanel == treePanel,
		filename:   filename,
		line:       line + 1,
		col:        col + 1,
		dirty:      m.dirty,
		errMsg:     m.errMsg,
	}

	// Input overlay on status bar
	statusView := sb.View()
	if m.inputMode == inputNewNote {
		statusView = m.newNoteView()
	} else if m.inputMode == inputConfirmDelete {
		node := m.tree.selectedNode()
		name := ""
		if node != nil {
			name = node.Name
		}
		statusView = statusBarStyle.Width(m.width).Render(
			fmt.Sprintf("Delete %s? (y/n)", name))
	} else if m.inputMode == inputUnsavedGuard {
		statusView = statusBarStyle.Width(m.width).Render(
			"Unsaved changes. Save? (y/n/Esc)")
	}

	return lipgloss.JoinVertical(lipgloss.Left, mainView, statusView)
}

func (m model) newNoteView() string {
	return statusBarStyle.Width(m.width).Render(
		"New note: " + m.newNoteInput.View())
}

func (m model) filterView() string {
	var b strings.Builder
	b.WriteString(m.filterInput.View())
	b.WriteString("\n")

	maxVisible := m.treeHeight - 1
	if maxVisible < 0 {
		maxVisible = 0
	}
	start := m.filterOffset
	end := start + maxVisible
	if end > len(m.filterResults) {
		end = len(m.filterResults)
	}

	for i := start; i < end; i++ {
		line := m.filterResults[i].Name
		if i == m.filterCursor {
			line = treeSelectedStyle.Render(line)
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return treePanelStyle.Width(m.treeWidth).Height(m.treeHeight).Render(b.String())
}
```

- [ ] **Step 2: Update main.go to run the TUI**

Replace `main.go` with:

```go
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type setupModel struct {
	input   textinput.Model
	errMsg  string
	done    bool
	vault   string
}

func newSetupModel() setupModel {
	ti := textinput.New()
	ti.Placeholder = "/home/user/notes"
	ti.CharLimit = 512
	ti.Width = 50
	ti.Focus()
	return setupModel{input: ti}
}

func (m setupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			path := m.input.Value()
			if path == "" {
				m.errMsg = "Please enter a path"
				return m, nil
			}
			// Expand ~
			if len(path) > 0 && path[0] == '~' {
				home, _ := os.UserHomeDir()
				path = home + path[1:]
			}
			// Check or create
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				if err := os.MkdirAll(path, 0o755); err != nil {
					m.errMsg = fmt.Sprintf("Cannot create: %v", err)
					return m, nil
				}
			} else if err != nil {
				m.errMsg = fmt.Sprintf("Error: %v", err)
				return m, nil
			} else if !info.IsDir() {
				m.errMsg = "Path is not a directory"
				return m, nil
			}

			cfg := Config{Vault: path}
			if err := saveConfig(cfg); err != nil {
				m.errMsg = fmt.Sprintf("Save config failed: %v", err)
				return m, nil
			}
			m.vault = path
			m.done = true
			return m, tea.Quit

		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m setupModel) View() string {
	s := lipgloss.NewStyle().Padding(1, 2)
	title := lipgloss.NewStyle().Bold(true).Render("Welcome to Clipad!")
	prompt := "\nEnter your vault path:"
	input := "\n\n" + m.input.View()
	hint := "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		"The directory will be created if it doesn't exist. Press Enter to confirm.")
	errView := ""
	if m.errMsg != "" {
		errView = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errMsg)
	}
	return s.Render(title + prompt + input + hint + errView)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		// First run — setup
		setup := newSetupModel()
		p := tea.NewProgram(setup, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		sm := result.(setupModel)
		if !sm.done {
			os.Exit(0)
		}
		cfg = Config{Vault: sm.vault}
	}

	// Validate vault exists
	if _, err := os.Stat(cfg.Vault); err != nil {
		fmt.Fprintf(os.Stderr, "Vault directory not found: %s\n", cfg.Vault)
		os.Exit(1)
	}

	m := newModel(cfg.Vault)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Verify build compiles**

```bash
go build -o clipad .
```
Expected: compiles without errors, produces `clipad` binary.

- [ ] **Step 4: Manual smoke test**

```bash
# Create a temp vault for testing
mkdir -p /tmp/clipad-test/notes/daily
echo "# Welcome" > /tmp/clipad-test/welcome.md
echo "# Daily" > /tmp/clipad-test/notes/daily/today.md

# Set config pointing to it
mkdir -p ~/.config/clipad
echo 'vault = "/tmp/clipad-test"' > ~/.config/clipad/config.toml

# Run the app
./clipad
```

Verify:
- Tree shows on the left with folders and files
- Tab switches between tree and editor
- Enter opens a file
- Text editing works in the editor
- Ctrl+S saves
- Ctrl+N creates a new note
- Ctrl+P toggles preview
- Ctrl+D (in tree) shows delete confirmation
- Ctrl+Q quits
- `/` in tree activates filter
- Status bar shows keybindings and file info

- [ ] **Step 5: Commit**

```bash
git add model.go main.go
git commit -m "feat: wire up main model with layout, keybindings, and all components"
```

---

### Task 9: Polish and Edge Cases

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add initial note on empty vault**

In `main.go`, after vault validation, if the vault has no `.md` files (recursively), create a welcome note:

Add this before `m := newModel(cfg.Vault)`:

```go
// Create welcome note if vault has no markdown files
root, err := buildTree(cfg.Vault)
if err == nil && len(collectFiles(root)) == 0 {
	os.WriteFile(filepath.Join(cfg.Vault, "welcome.md"),
		[]byte("# Welcome to Clipad\n\nStart writing your notes here.\n"), 0o644)
}
```

- [ ] **Step 2: Add .clipad binary to .gitignore**

Create `.gitignore`:

```
clipad
```

- [ ] **Step 3: Verify build and smoke test again**

```bash
go build -o clipad . && ./clipad
```

- [ ] **Step 4: Commit**

```bash
git add main.go .gitignore
git commit -m "feat: add welcome note for empty vaults and gitignore"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run all tests**

```bash
go test -v ./...
```
Expected: all tests pass.

- [ ] **Step 2: Run go vet**

```bash
go vet ./...
```
Expected: no issues.

- [ ] **Step 3: Full manual test of all features**

Test checklist:
- [ ] First-run setup flow (delete config, re-run)
- [ ] File tree navigation (up/down, expand/collapse)
- [ ] Open file (Enter)
- [ ] Edit text, verify dirty flag shows `[+]`
- [ ] Save (Ctrl+S), verify `[+]` disappears
- [ ] New note (Ctrl+N), enter path with subdirectory
- [ ] New note with existing name opens the file
- [ ] Delete (Ctrl+D in tree), confirm with y
- [ ] Delete cancel with n
- [ ] Preview toggle (Ctrl+P), verify markdown renders
- [ ] Panel switching (Tab)
- [ ] Fuzzy filter (/ in tree), type to filter, Enter to open, Esc to cancel
- [ ] Unsaved changes guard: edit, then Ctrl+Q, test y/n/Esc
- [ ] Unsaved changes guard: edit, then switch file via tree
- [ ] Resize terminal, verify layout adapts
- [ ] Small terminal (<60x15), verify "too small" message

- [ ] **Step 4: Commit any fixes from manual testing**

```bash
git add -A && git commit -m "fix: address issues found during manual testing"
```
(Only if changes were needed.)
