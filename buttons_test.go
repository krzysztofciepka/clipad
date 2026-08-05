package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newBarTestModel builds a fully initialised model the way the app does.
// recalcLayout touches the tree, the editor and the image registry, so tests
// that call it need real state rather than a zero-value struct. The env vars
// keep loadShortcuts away from the developer's real config, matching
// tree_toggle_test.go.
func newBarTestModel(t *testing.T, width, height int) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	m := newModel(t.TempDir(), nil, "", "")
	m.width = width
	m.height = height
	m.recalcLayout()
	return m
}

func TestButtonBarHeight(t *testing.T) {
	tests := []struct {
		name                   string
		treeWidth, availHeight int
		collapsed              bool
		n                      int
		want                   int
	}{
		{"expanded four buttons", 20, 30, false, 4, 5},
		{"collapsed", 20, 30, true, 4, 1},
		{"tree hidden", 0, 30, false, 4, 0},
		{"negative tree width", -1, 30, false, 4, 0},
		{"no room at all", 20, 1, false, 4, 0},
		{"squeezed falls back to rule row", 20, 5, false, 4, 1},
		{"exactly fits", 20, 6, false, 4, 5},
		{"collapsed still fits tiny", 20, 2, true, 4, 1},
		{"two buttons", 20, 30, false, 2, 3},
		{"one button", 20, 30, false, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buttonBarHeight(tt.treeWidth, tt.availHeight, tt.collapsed, tt.n)
			if got != tt.want {
				t.Errorf("buttonBarHeight(%d,%d,%v,%d) = %d, want %d",
					tt.treeWidth, tt.availHeight, tt.collapsed, tt.n, got, tt.want)
			}
		})
	}
}

func TestBarButtons(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(m *model)
		wantLabels []string
	}{
		{
			name:       "default set",
			setup:      func(m *model) { m.currentFile = "/v/a.md" },
			wantLabels: []string{"Find & replace", "AI tools", "Ask", "AI search"},
		},
		{
			name:       "diff mode",
			setup:      func(m *model) { m.inputMode = inputPluginDiff },
			wantLabels: []string{"Approve", "Reject"},
		},
		{
			name:       "review mode",
			setup:      func(m *model) { m.inputMode = inputPluginReview },
			wantLabels: []string{"Copy"},
		},
		{
			name:       "chat mode",
			setup:      func(m *model) { m.chatOpen = true },
			wantLabels: []string{"Input", "Prev cite", "Next cite"},
		},
		{
			name: "diff wins over chat",
			setup: func(m *model) {
				m.chatOpen = true
				m.inputMode = inputPluginDiff
			},
			wantLabels: []string{"Approve", "Reject"},
		},
		{
			name:       "modal overlay keeps the default set",
			setup:      func(m *model) { m.inputMode = inputVaultSearch },
			wantLabels: []string{"Find & replace", "AI tools", "Ask", "AI search"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{}
			tt.setup(&m)
			got := m.barButtons()
			if len(got) != len(tt.wantLabels) {
				t.Fatalf("got %d buttons, want %d: %v", len(got), len(tt.wantLabels), got)
			}
			for i, want := range tt.wantLabels {
				if got[i].label != want {
					t.Errorf("button %d = %q, want %q", i, got[i].label, want)
				}
			}
		})
	}
}

func TestBarButtonsEnablement(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(m *model)
		label   string
		enabled bool
	}{
		{"find & replace needs an open note", func(m *model) {}, "Find & replace", false},
		{"find & replace with open file", func(m *model) { m.currentFile = "/v/a.md" }, "Find & replace", true},
		{"find & replace with new note", func(m *model) { m.newNoteDir = "/v" }, "Find & replace", true},
		{"ai tools needs an open note", func(m *model) {}, "AI tools", false},
		{"ai tools with open file", func(m *model) { m.currentFile = "/v/a.md" }, "AI tools", true},
		{"ask is always live", func(m *model) {}, "Ask", true},
		{"ai search is always live", func(m *model) {}, "AI search", true},
		{
			"citation buttons need citations",
			func(m *model) { m.chatOpen = true },
			"Next cite", false,
		},
		{
			"citation buttons live once cited",
			func(m *model) {
				m.chatOpen = true
				m.chatTurns = []chatTurn{{Role: "assistant", Citations: []citation{{Path: "a.md", StartLine: 1, EndLine: 2}}}}
			},
			"Next cite", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{}
			tt.setup(&m)
			for _, b := range m.barButtons() {
				if b.label == tt.label {
					if b.enabled != tt.enabled {
						t.Errorf("%q enabled = %v, want %v", tt.label, b.enabled, tt.enabled)
					}
					return
				}
			}
			t.Fatalf("button %q not present", tt.label)
		})
	}
}

func TestBarInert(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *model)
		want  bool
	}{
		{"normal", func(m *model) {}, false},
		{"diff", func(m *model) { m.inputMode = inputPluginDiff }, false},
		{"review", func(m *model) { m.inputMode = inputPluginReview }, false},
		{"vault search modal", func(m *model) { m.inputMode = inputVaultSearch }, true},
		{"shortcut picker", func(m *model) { m.inputMode = inputShortcutSelect }, true},
		{"tree filter", func(m *model) { m.inputMode = inputFilter }, true},
		{"streaming plugin run", func(m *model) { m.pluginProcessing = true }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{}
			tt.setup(&m)
			if got := m.barInert(); got != tt.want {
				t.Errorf("barInert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncBarLayoutShrinksTree(t *testing.T) {
	tests := []struct {
		name           string
		collapsed      bool
		treeHidden     bool
		wantBar        int
		wantTreeHeight int
	}{
		{"expanded takes five rows", false, false, 5, 40 - 2 - 5},
		{"collapsed takes one row", true, false, 1, 40 - 2 - 1},
		{"tree hidden takes none", false, true, 0, 40 - 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBarTestModel(t, 120, 40)
			m.buttonsCollapsed = tt.collapsed
			m.treeHidden = tt.treeHidden
			m.recalcLayout()
			if m.barHeight != tt.wantBar {
				t.Errorf("barHeight = %d, want %d", m.barHeight, tt.wantBar)
			}
			if m.treeHeight != tt.wantTreeHeight {
				t.Errorf("treeHeight = %d, want %d", m.treeHeight, tt.wantTreeHeight)
			}
			if m.tree.height != m.treeHeight {
				t.Errorf("tree.height = %d, want %d", m.tree.height, m.treeHeight)
			}
			if m.barHeight > 0 && m.treeHeight+m.barHeight != m.height-2 {
				t.Errorf("left column = %d rows, want %d", m.treeHeight+m.barHeight, m.height-2)
			}
		})
	}
}

func TestSyncBarLayoutDropsFocusWhenBarDisappears(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.buttonFocused = true
	m.buttonCursor = 3
	m.treeHidden = true
	m.recalcLayout()
	if m.buttonFocused {
		t.Error("focus should drop when the bar has no rows")
	}
}

func TestClampButtonCursor(t *testing.T) {
	tests := []struct {
		name      string
		barHeight int
		cursor    int
		want      int
	}{
		{"in range", 5, 3, 3},
		{"past the end", 5, 9, 4},
		{"negative", 5, -2, 0},
		{"no bar", 0, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{barHeight: tt.barHeight, buttonCursor: tt.cursor}
			m.clampButtonCursor()
			if m.buttonCursor != tt.want {
				t.Errorf("buttonCursor = %d, want %d", m.buttonCursor, tt.want)
			}
		})
	}
}

func TestButtonBarViewRowCount(t *testing.T) {
	rows := []barButton{
		{label: "Find & replace", action: actionFindReplace, enabled: true},
		{label: "AI tools", action: actionAITools, enabled: true},
	}
	tests := []struct {
		name    string
		height  int
		wantLen int
	}{
		{"expanded", 3, 3},
		{"collapsed", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buttonBarView(rows, tt.height, 20, false, false, 0)
			if got := len(strings.Split(out, "\n")); got != tt.wantLen {
				t.Errorf("rendered %d lines, want %d:\n%s", got, tt.wantLen, out)
			}
		})
	}
}

func TestButtonBarViewToggleGlyph(t *testing.T) {
	rows := []barButton{{label: "Ask", action: actionAsk, enabled: true}}
	expanded := buttonBarView(rows, 2, 20, false, false, 0)
	if !strings.Contains(expanded, "[-]") {
		t.Errorf("expanded bar should show [-]:\n%s", expanded)
	}
	collapsed := buttonBarView(rows, 1, 20, false, false, 0)
	if !strings.Contains(collapsed, "[+]") {
		t.Errorf("collapsed bar should show [+]:\n%s", collapsed)
	}
}

func TestButtonBarViewRuleFillsWidth(t *testing.T) {
	rows := []barButton{{label: "Ask", action: actionAsk, enabled: true}}
	out := buttonBarView(rows, 2, 22, false, false, 0)
	rule := strings.Split(out, "\n")[0]
	if strings.Contains(rule, "…") {
		t.Errorf("the rule is a divider, not text — it must not be ellipsised: %q", rule)
	}
	// The rule sits inside the same Padding(0,1) as the tree rows above it,
	// so it runs to one column short of the divider.
	if !strings.HasPrefix(rule, " ─[-]─") || !strings.HasSuffix(rule, "─ │") {
		t.Errorf("the rule should span the padded content width: %q", rule)
	}
}

func TestButtonBarViewShowsLabels(t *testing.T) {
	rows := []barButton{
		{label: "Find & replace", action: actionFindReplace, enabled: true},
		{label: "AI tools", action: actionAITools, enabled: false},
	}
	out := buttonBarView(rows, 3, 24, false, false, 0)
	for _, want := range []string{"Find & replace", "AI tools"} {
		if !strings.Contains(out, want) {
			t.Errorf("bar is missing %q:\n%s", want, out)
		}
	}
}

func TestButtonBarViewTruncatesLongLabels(t *testing.T) {
	rows := []barButton{{label: "Find & replace", action: actionFindReplace, enabled: true}}
	const width = 10
	out := buttonBarView(rows, 2, width, false, false, 0)
	// The right border sits outside Width, exactly as treePanelStyle renders
	// the tree: content occupies `width` columns and the divider adds one.
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) != width+1 {
			t.Errorf("line is %d cols, want %d: %q", lipgloss.Width(line), width+1, line)
		}
	}
}

func TestButtonBarRowAt(t *testing.T) {
	// width 120, height 40 → treeWidth 30, status bar at y=39.
	// With four buttons: barHeight 5, treeHeight 33 → bar rows are y=33..37.
	tests := []struct {
		name    string
		mutate  func(m *model)
		x, y    int
		wantRow int
		wantOK  bool
	}{
		{"inside the tree", nil, 5, 10, 0, false},
		{"first bar row", nil, 5, 33, 0, true},
		{"a button row", nil, 5, 35, 2, true},
		{"last bar row", nil, 5, 37, 4, true},
		{"status bar row", nil, 5, 39, 0, false},
		{"editor column", nil, 60, 35, 0, false},
		{"tree border column", nil, 30, 35, 0, false},
		{"tree hidden", func(m *model) { m.treeHidden = true; m.recalcLayout() }, 5, 35, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBarTestModel(t, 120, 40)
			if tt.mutate != nil {
				tt.mutate(&m)
			}
			row, ok := m.buttonBarRowAt(tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && row != tt.wantRow {
				t.Errorf("row = %d, want %d", row, tt.wantRow)
			}
		})
	}
}

func TestActivateBarRowToggle(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	before := m.treeHeight

	m2, _ := m.activateBarRow(0)
	if !m2.buttonsCollapsed {
		t.Fatal("row 0 should collapse the bar")
	}
	if m2.barHeight != 1 {
		t.Errorf("barHeight = %d, want 1", m2.barHeight)
	}
	if m2.treeHeight <= before {
		t.Errorf("collapsing should give rows back to the tree: %d → %d", before, m2.treeHeight)
	}

	m3, _ := m2.activateBarRow(0)
	if m3.buttonsCollapsed {
		t.Error("a second toggle should expand the bar again")
	}
	if m3.treeHeight != before {
		t.Errorf("treeHeight = %d, want %d", m3.treeHeight, before)
	}
}

func TestActivateBarRowRunsActions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *model)
		row   int
		check func(t *testing.T, m model)
	}{
		{
			name:  "find & replace",
			setup: func(m *model) { m.currentFile = "/v/a.md" },
			row:   1,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputReplaceSearch {
					t.Errorf("inputMode = %v, want inputReplaceSearch", m.inputMode)
				}
			},
		},
		{
			name:  "disabled button does nothing",
			setup: func(m *model) {},
			row:   1,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputNone {
					t.Errorf("inputMode = %v, want inputNone", m.inputMode)
				}
			},
		},
		{
			name:  "reject in diff mode",
			setup: func(m *model) { m.inputMode = inputPluginDiff; m.pluginDiffResult = "x" },
			row:   2,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputNone || m.pluginDiffResult != "" {
					t.Error("reject should clear the run")
				}
			},
		},
		{
			name:  "row past the end",
			setup: func(m *model) { m.currentFile = "/v/a.md" },
			row:   99,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputNone {
					t.Error("out-of-range row should be a no-op")
				}
			},
		},
		{
			name:  "inert bar ignores activation",
			setup: func(m *model) { m.currentFile = "/v/a.md"; m.inputMode = inputVaultSearch },
			row:   1,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputVaultSearch {
					t.Error("a modal overlay must not be replaced by a button action")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBarTestModel(t, 120, 40)
			tt.setup(&m)
			m.recalcLayout()
			got, _ := m.activateBarRow(tt.row)
			tt.check(t, got)
		})
	}
}

func TestHandleButtonBarMouseIgnoresNonPress(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	for _, msg := range []tea.MouseMsg{
		{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
		{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
		{Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
	} {
		got, _ := handleButtonBarMouse(m, 0, msg)
		if got.(model).buttonsCollapsed {
			t.Errorf("%v should not toggle the bar", msg)
		}
	}
}

func TestHandleMouseMsgRoutesToBar(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	msg := tea.MouseMsg{X: 5, Y: m.treeHeight, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	got, _ := handleMouseMsg(m, msg)
	if !got.(model).buttonsCollapsed {
		t.Error("a click on the bar's toggle row should reach the bar, not the tree")
	}
}

func barFocusModel(t *testing.T) model {
	t.Helper()
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md" // enables Find & replace and AI tools
	m.buttonFocused = true
	return m
}

func TestHandleButtonKeysNotFocused(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	if _, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyDown}); handled {
		t.Error("keys must fall through when the bar is not focused")
	}
}

func TestHandleButtonKeysCursor(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		keys       []tea.KeyMsg
		wantCursor int
	}{
		{"down moves", 0, []tea.KeyMsg{{Type: tea.KeyDown}}, 1},
		{"up moves", 2, []tea.KeyMsg{{Type: tea.KeyUp}}, 1},
		{"up stops at the toggle row", 0, []tea.KeyMsg{{Type: tea.KeyUp}}, 0},
		{"down stops at the last row", 4, []tea.KeyMsg{{Type: tea.KeyDown}}, 4},
		{"walks the whole bar", 0, []tea.KeyMsg{
			{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyDown},
		}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := barFocusModel(t)
			m.buttonCursor = tt.start
			for _, k := range tt.keys {
				mm, _, handled := m.handleButtonKeys(k)
				if !handled {
					t.Fatalf("key %v not handled", k)
				}
				m = mm.(model)
			}
			if m.buttonCursor != tt.wantCursor {
				t.Errorf("buttonCursor = %d, want %d", m.buttonCursor, tt.wantCursor)
			}
		})
	}
}

func TestHandleButtonKeysActivate(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeySpace}} {
		m := barFocusModel(t)
		m.buttonCursor = 1 // Find & replace
		mm, _, handled := m.handleButtonKeys(key)
		if !handled {
			t.Fatalf("%v not handled", key)
		}
		got := mm.(model)
		if got.inputMode != inputReplaceSearch {
			t.Errorf("%v: inputMode = %v, want inputReplaceSearch", key, got.inputMode)
		}
		if got.buttonFocused {
			t.Errorf("%v: focus should leave the bar when a modal opens", key)
		}
	}
}

func TestHandleButtonKeysEscReturnsToTree(t *testing.T) {
	m := barFocusModel(t)
	m.buttonCursor = 2
	mm, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatal("esc not handled")
	}
	got := mm.(model)
	if got.buttonFocused {
		t.Error("esc should drop bar focus")
	}
	if got.activePanel != treePanel {
		t.Errorf("activePanel = %v, want treePanel", got.activePanel)
	}
}

func TestHandleButtonKeysSwallowsOtherKeys(t *testing.T) {
	m := barFocusModel(t)
	m.editor.SetValue("")
	mm, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !handled {
		t.Fatal("stray keys must be swallowed, not passed to the editor")
	}
	if got := mm.(model).editor.Value(); got != "" {
		t.Errorf("editor value = %q, want empty", got)
	}
}

func TestHandleButtonKeysCtrlQFallsThrough(t *testing.T) {
	m := barFocusModel(t)
	if _, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyCtrlQ}); handled {
		t.Error("ctrl+q must reach the global quit guard")
	}
}

func TestTabCycleNormalMode(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md"
	m.activePanel = treePanel

	// tree → bar
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if !m.buttonFocused {
		t.Fatal("tab from the tree should focus the bar")
	}
	if m.buttonCursor != 0 {
		t.Errorf("buttonCursor = %d, want 0", m.buttonCursor)
	}

	// bar → editor
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.buttonFocused || m.activePanel != editorPanel {
		t.Fatalf("tab from the bar should focus the editor: focused=%v panel=%v",
			m.buttonFocused, m.activePanel)
	}

	// editor → tree
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.activePanel != treePanel || m.buttonFocused {
		t.Fatalf("tab from the editor should return to the tree: panel=%v", m.activePanel)
	}
}

func TestTabCycleSkipsHiddenBar(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md"
	m.treeHidden = true
	m.recalcLayout()
	m.activePanel = treePanel
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := mm.(model)
	if got.buttonFocused {
		t.Error("there is no bar to focus when the tree is hidden")
	}
	if got.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", got.activePanel)
	}
}

func TestTabCycleDiffMode(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md"
	m.inputMode = inputPluginDiff
	m.paneFocus = paneFocusRight
	m.recalcLayout()

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.paneFocus != paneFocusLeft || m.buttonFocused {
		t.Fatalf("right → left expected, got focus=%v bar=%v", m.paneFocus, m.buttonFocused)
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if !m.buttonFocused {
		t.Fatal("left → bar expected")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.buttonFocused || m.paneFocus != paneFocusRight {
		t.Fatalf("bar → right expected, got focus=%v bar=%v", m.paneFocus, m.buttonFocused)
	}
}

// TestTreeScrollsInsideReducedHeight covers the task's core layout
// requirement: the tree must scroll within whatever room the bar leaves it,
// never render over the bar.
func TestTreeScrollsInsideReducedHeight(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	for i := 0; i < 60; i++ {
		p := filepath.Join(vault, fmt.Sprintf("note-%02d.md", i))
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newModel(vault, nil, "", "")
	m.width, m.height = 120, 30
	m.recalcLayout()

	if len(m.tree.items) <= m.treeHeight {
		t.Fatalf("precondition: need more items (%d) than tree rows (%d)", len(m.tree.items), m.treeHeight)
	}

	// The rendered left column must be exactly the tree height plus the bar,
	// never more — an overflowing tree would push the bar off screen.
	treeRows := len(strings.Split(m.tree.View(true), "\n"))
	if treeRows != m.treeHeight {
		t.Errorf("tree rendered %d rows, want %d", treeRows, m.treeHeight)
	}

	// Scrolling still works inside the reduced height.
	before := m.tree.offset
	m.tree.scrollBy(10)
	if m.tree.offset == before {
		t.Error("tree should scroll within the space the bar leaves it")
	}
	maxOffset := len(m.tree.items) - m.tree.itemsHeight()
	m.tree.scrollBy(1000)
	if m.tree.offset != maxOffset {
		t.Errorf("offset = %d, want clamped to %d", m.tree.offset, maxOffset)
	}
}
