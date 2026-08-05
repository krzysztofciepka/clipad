package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// barAction identifies what a button bar row does. The row→action mapping is
// data rather than closures so it stays comparable in tests.
type barAction int

const (
	actionToggleBar barAction = iota
	actionFindReplace
	actionAITools
	actionAsk
	actionAISearch
	actionApprove
	actionReject
	actionCopyReview
	actionChatInput
	actionPrevCite
	actionNextCite
)

// barButton is one clickable row of the bar. A disabled button renders dim and
// ignores clicks and Enter; it is used for actions whose guard would make them
// a silent no-op anyway.
type barButton struct {
	label   string
	action  barAction
	enabled bool
}

// noteOpen reports whether there is a buffer to act on — an existing file or an
// unsaved new note. Several actions are meaningless without one.
func (m model) noteOpen() bool {
	return m.currentFile != "" || m.newNoteDir != ""
}

// barButtons returns the button set for the current mode. Diff and review are
// checked before the chat panel: a plugin run started from the chat panel puts
// the diff on screen, and approve/reject is what matters then.
func (m model) barButtons() []barButton {
	switch m.inputMode {
	case inputPluginDiff:
		return []barButton{
			{label: "Approve", action: actionApprove, enabled: true},
			{label: "Reject", action: actionReject, enabled: true},
		}
	case inputPluginReview:
		return []barButton{
			{label: "Copy", action: actionCopyReview, enabled: true},
		}
	}
	if m.chatOpen {
		cited := len(chatCitations(m.chatTurns)) > 0
		return []barButton{
			{label: "Input", action: actionChatInput, enabled: true},
			{label: "Prev cite", action: actionPrevCite, enabled: cited},
			{label: "Next cite", action: actionNextCite, enabled: cited},
		}
	}
	open := m.noteOpen()
	return []barButton{
		{label: "Find & replace", action: actionFindReplace, enabled: open},
		{label: "AI tools", action: actionAITools, enabled: open},
		// Ask and AI search stay live without an open note: Ask works against
		// the whole vault, and AI search reports its own configuration error.
		{label: "Ask", action: actionAsk, enabled: true},
		{label: "AI search", action: actionAISearch, enabled: true},
	}
}

// barInert reports whether the bar is currently non-interactive. A modal
// overlay other than the diff and review views owns the keyboard and already
// swallows mouse events, and a streaming plugin run must not be accepted
// halfway. The bar renders dim in those states rather than looking live.
func (m model) barInert() bool {
	if m.pluginProcessing {
		return true
	}
	switch m.inputMode {
	case inputNone, inputPluginDiff, inputPluginReview:
		return false
	}
	return true
}

// clampButtonCursor keeps the focus cursor inside the bar's current rows.
func (m *model) clampButtonCursor() {
	if m.barHeight <= 0 {
		m.buttonCursor = 0
		return
	}
	if m.buttonCursor >= m.barHeight {
		m.buttonCursor = m.barHeight - 1
	}
	if m.buttonCursor < 0 {
		m.buttonCursor = 0
	}
}

// syncBarLayout recomputes the bar height and the tree height it takes rows
// from. The active button set depends on the mode, so this runs on every mode
// switch — it is the cheap part of recalcLayout, without the viewport rebuilds
// that would reset diff and chat scroll positions on every transition.
func (m *model) syncBarLayout() {
	avail := m.height - 2
	m.barHeight = buttonBarHeight(m.treeWidth, avail, m.buttonsCollapsed, len(m.barButtons()))
	m.treeHeight = avail - m.barHeight
	if m.treeHeight < 1 {
		m.treeHeight = 1
	}
	m.tree.height = m.treeHeight
	m.tree.clampOffset()
	if m.barHeight == 0 {
		m.buttonFocused = false
	}
	m.clampButtonCursor()
}

var (
	// buttonBarStyle mirrors treePanelStyle so the vertical divider between
	// the left pane and the editor runs unbroken past the bar.
	buttonBarStyle = lipgloss.NewStyle().
			Padding(0, 1).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder())

	buttonRuleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	buttonLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	buttonDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// buttonBarView renders the bar at exactly `height` rows and `width` columns
// (plus its right border), so the left column always joins flush with the
// editor. Row 0 is the toggle rule; rows 1.. are buttons, of which only
// height-1 fit. `inert` dims everything, `focused`+`cursor` highlight one row.
func buttonBarView(rows []barButton, height, width int, inert, focused bool, cursor int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	maxW := width - 2 // buttonBarStyle's horizontal padding
	if maxW < 1 {
		maxW = 1
	}

	fit := func(s string) string {
		if lipgloss.Width(s) > maxW {
			return ansi.Truncate(s, maxW, "…")
		}
		return s
	}
	pad := func(s string) string {
		if w := lipgloss.Width(s); w < maxW {
			return s + strings.Repeat(" ", maxW-w)
		}
		return s
	}

	// The toggle glyph reflects what is on screen: a bar squeezed down to its
	// rule row by a short terminal reads as collapsed, because it is.
	glyph := "[+]"
	if height > 1 {
		glyph = "[-]"
	}
	// The rule fills the pane with box-drawing characters rather than being
	// ellipsised — it is a divider, not text.
	rule := "─" + glyph
	if w := lipgloss.Width(rule); w < maxW {
		rule += strings.Repeat("─", maxW-w)
	} else {
		rule = ansi.Truncate(rule, maxW, "")
	}

	lines := make([]string, 0, height)
	renderRow := func(i int, text string, enabled bool) {
		switch {
		case focused && i == cursor:
			text = treeSelectedStyle.Render(pad(text))
		case inert || !enabled:
			text = buttonDisabledStyle.Render(text)
		case i == 0:
			text = buttonRuleStyle.Render(text)
		default:
			text = buttonLabelStyle.Render(text)
		}
		lines = append(lines, text)
	}
	renderRow(0, rule, true)
	for i := 0; i < height-1 && i < len(rows); i++ {
		renderRow(i+1, fit(" "+rows[i].label), rows[i].enabled)
	}

	return buttonBarStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

// handleButtonKeys handles a keystroke while the bar has keyboard focus. The
// bool reports whether the key was consumed; false means the caller should run
// its normal routing. Every unrecognised key is consumed on purpose — without
// that, a keystroke aimed at the bar would fall through to the editor and type
// into the buffer.
func (m model) handleButtonKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if !m.buttonFocused {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+q":
		return m, nil, false // the global quit guard owns this
	case "up":
		if m.buttonCursor > 0 {
			m.buttonCursor--
		}
		return m, nil, true
	case "down":
		if m.buttonCursor < m.barHeight-1 {
			m.buttonCursor++
		}
		return m, nil, true
	case "enter", " ":
		mm, cmd := m.activateBarRow(m.buttonCursor)
		return mm, cmd, true
	case "esc":
		m.buttonFocused = false
		m.activePanel = treePanel
		m.editor.Blur()
		return m, nil, true
	case "tab":
		m.buttonFocused = false
		switch m.inputMode {
		case inputPluginDiff, inputPluginReview:
			m.paneFocus = paneFocusRight
			return m, nil, true
		}
		m.activePanel = editorPanel
		if m.editorMode == modeEdit && m.noteOpen() {
			return m, m.editor.Focus(), true
		}
		return m, nil, true
	}
	return m, nil, true
}

// buttonBarRowAt maps an absolute terminal coordinate to a bar row. The left
// column is split at m.treeHeight: above is the file tree, below is the bar.
func (m model) buttonBarRowAt(x, y int) (int, bool) {
	if m.barHeight <= 0 {
		return 0, false
	}
	hit, _, localY, ok := hitTestPanel(m.treeWidth, m.chatWidth, m.width, m.height, x, y)
	if !ok || hit != treePanel || localY < m.treeHeight {
		return 0, false
	}
	row := localY - m.treeHeight
	if row >= m.barHeight {
		return 0, false
	}
	return row, true
}

// handleButtonBarMouse routes a mouse event that landed on the bar. Only a
// left-button press activates; wheel events over the bar are ignored rather
// than scrolling the tree behind it.
func handleButtonBarMouse(m model, row int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	mm, cmd := m.activateBarRow(row)
	return mm, cmd
}

// activateBarRow runs the row's action. Row 0 is the collapse toggle.
func (m model) activateBarRow(row int) (model, tea.Cmd) {
	if m.barInert() {
		return m, nil
	}
	if row == 0 {
		m.buttonsCollapsed = !m.buttonsCollapsed
		m.syncBarLayout()
		return m, nil
	}
	rows := m.barButtons()
	i := row - 1
	if i < 0 || i >= len(rows) || !rows[i].enabled {
		return m, nil
	}
	return m.runBarAction(rows[i].action)
}

// runBarAction dispatches to the shared action methods in actions.go, so a
// button and its keyboard shortcut run exactly the same code.
func (m model) runBarAction(a barAction) (model, tea.Cmd) {
	var cmd tea.Cmd
	switch a {
	case actionFindReplace:
		cmd = m.startFindReplace()
	case actionAITools:
		cmd = m.openShortcutPicker()
	case actionAsk:
		cmd = m.toggleChat()
	case actionAISearch:
		cmd = m.openVaultSearch()
	case actionApprove:
		cmd = m.acceptDiff()
	case actionReject:
		cmd = m.rejectDiff()
	case actionCopyReview:
		cmd = m.copyReview()
	case actionChatInput:
		cmd = m.chatFocusInput()
	case actionPrevCite:
		cmd = m.jumpCitation(-1)
	case actionNextCite:
		cmd = m.jumpCitation(1)
	}
	// Opening a modal makes the bar inert, and several actions move focus
	// elsewhere; either way the bar must not keep keyboard focus.
	if m.barInert() || m.barHeight == 0 {
		m.buttonFocused = false
	}
	m.syncBarLayout()
	return m, cmd
}

// buttonBarHeight returns how many rows the bar occupies. availHeight is the
// height of the left column (terminal height minus the status bar). The bar
// falls back to its single toggle row when the expanded form would leave the
// file tree no room at all.
func buttonBarHeight(treeWidth, availHeight int, collapsed bool, n int) int {
	if treeWidth <= 0 || availHeight < 2 {
		return 0
	}
	if collapsed {
		return 1
	}
	h := 1 + n
	if h > availHeight-1 {
		return 1
	}
	return h
}
