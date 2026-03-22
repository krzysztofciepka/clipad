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
	pendingNewNote
)

type inputMode int

const (
	inputNone inputMode = iota
	inputFilter
	inputConfirmDelete
	inputUnsavedGuard
	inputPluginSelect
	inputPluginConfig
	inputPluginPrompt
	inputPluginDiff
)

type model struct {
	width  int
	height int

	activePanel panel
	editorMode  editorMode

	vault string

	tree       TreePanel
	treeRoot   *TreeNode
	treeWidth  int
	treeHeight int

	editor       textarea.Model
	editorWidth  int
	editorHeight int

	preview viewport.Model

	currentFile  string
	cleanContent string // content as last saved/loaded — dirty = editor differs from this
	newNoteDir   string // non-empty when editing a new unsaved note; holds the target directory

	inputMode     inputMode
	filterInput   textinput.Model
	filterResults []*TreeNode
	filterCursor  int
	filterOffset  int

	pendingAction     pendingActionType
	pendingSwitchPath string

	errMsg string

	// Plugin system
	plugins            []Plugin
	pluginCursor       int
	pluginActive       Plugin
	pluginPromptInput  textinput.Model
	pluginConfigFields []ConfigField
	pluginConfigIndex  int
	pluginConfigValues map[string]string
	pluginConfigInput  textinput.Model
	pluginDiffOriginal string
	pluginDiffResult   string
	pluginDiffViewL    viewport.Model
	pluginDiffViewR    viewport.Model
	pluginProcessing   bool
}

func newModel(vault string, plugins []Plugin) model {
	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.CharLimit = 256

	pi := textinput.New()
	pi.Placeholder = "Enter prompt..."
	pi.CharLimit = 500

	m := model{
		vault:             vault,
		activePanel:       treePanel,
		editorMode:        modeEdit,
		editor:            newEditor(),
		filterInput:       fi,
		plugins:           plugins,
		pluginPromptInput: pi,
	}

	root, err := buildTree(vault)
	if err != nil {
		m.errMsg = fmt.Sprintf("Error reading vault: %v", err)
	} else {
		m.treeRoot = root
		m.tree.root = root
		m.tree.rebuildItems()
	}

	return m
}

func (m model) isDirty() bool {
	return m.editor.Value() != m.cleanContent
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, warmRendererCmd())
}

func warmRendererCmd() tea.Cmd {
	return func() tea.Msg {
		getRenderer(80) // pre-warm with a reasonable default width
		return nil
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case pluginResultMsg:
		m.pluginProcessing = false
		if msg.err != nil {
			m.errMsg = "Plugin error: " + msg.err.Error()
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			return m, nil
		}
		if msg.result == m.pluginDiffOriginal {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			return m, nil
		}
		m.pluginDiffResult = msg.result
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, msg.result, m.editorWidth, m.editorHeight)
		m.inputMode = inputPluginDiff
		return m, nil

	case tea.KeyMsg:
		if m.pluginProcessing {
			return m, nil
		}

		if m.inputMode != inputNone {
			return m.handleInputMode(msg)
		}

		switch msg.String() {
		case "ctrl+q", "ctrl+c":
			if m.isDirty() {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingQuit
				return m, nil
			}
			return m, tea.Quit

		case "ctrl+s":
			m.saveCurrentFile()
			return m, nil

		case "ctrl+n":
			if m.isDirty() {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingNewNote
				return m, nil
			}
			m.startNewNote()
			return m, nil

		case "ctrl+p":
			return m.togglePreview()

		case "ctrl+@":
			if m.currentFile != "" || m.newNoteDir != "" {
				if len(m.plugins) > 0 {
					m.inputMode = inputPluginSelect
					m.pluginCursor = 0
				}
			}
			return m, nil

		case "tab":
			if m.activePanel == treePanel {
				m.activePanel = editorPanel
				cmd := m.editor.Focus()
				return m, cmd
			}
			m.activePanel = treePanel
			m.editor.Blur()
			return m, nil
		}

		if m.activePanel == treePanel {
			return m.handleTreeKeys(msg)
		}
		return m.handleEditorKeys(msg)
	}

	if m.activePanel == editorPanel && m.editorMode == modeEdit {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) handleTreeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.tree.moveUp()
		m.previewSelectedFile()
	case "down", "j":
		m.tree.moveDown()
		m.previewSelectedFile()
	case "enter":
		node := m.tree.toggleOrSelect()
		if node != nil {
			if m.isDirty() {
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
		cmd := m.filterInput.Focus()
		m.filterResults = collectFiles(m.treeRoot)
		m.filterCursor = 0
		m.filterOffset = 0
		return m, cmd
	case "ctrl+d":
		node := m.tree.selectedNode()
		if node != nil && !node.IsDir {
			m.inputMode = inputConfirmDelete
		}
	default:
		// Auto-switch to editor on printable input when a file is open
		if m.currentFile != "" && msg.Type == tea.KeyRunes {
			m.activePanel = editorPanel
			m.editor.Focus()
			return m.handleEditorKeys(msg)
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
	}
	return m, cmd
}

func (m model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.inputMode {
	case inputFilter:
		return m.handleFilterInput(msg)
	case inputConfirmDelete:
		return m.handleDeleteConfirm(msg)
	case inputUnsavedGuard:
		return m.handleUnsavedGuard(msg)
	case inputPluginSelect:
		return m.handlePluginSelect(msg)
	case inputPluginConfig:
		return m.handlePluginConfig(msg)
	case inputPluginPrompt:
		return m.handlePluginPrompt(msg)
	case inputPluginDiff:
		return m.handlePluginDiff(msg)
	}
	return m, nil
}


func (m model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.filterCursor < len(m.filterResults) {
			path := m.filterResults[m.filterCursor].Path
			m.inputMode = inputNone
			if m.isDirty() {
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
	case "esc", "ctrl+c":
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
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	case "ctrl+s":
		m.saveCurrentFile()
		return m, nil
	case "ctrl+p":
		m.inputMode = inputNone
		return m.togglePreview()
	case "ctrl+n":
		m.inputMode = inputNone
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingNewNote
			return m, nil
		}
		m.startNewNote()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
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
					m.cleanContent = ""
				}
				m.refreshTree()
			}
		}
		m.inputMode = inputNone
	case "n", "esc", "ctrl+c":
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
		m.cleanContent = m.editor.Value()
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
	case pendingNewNote:
		m.pendingAction = pendingNone
		m.startNewNote()
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
	m.cleanContent = string(data)
	m.editorMode = modeEdit
	m.tree.currentFile = path
	m.errMsg = ""
}

func (m *model) previewSelectedFile() {
	node := m.tree.selectedNode()
	if node == nil || node.IsDir {
		return
	}
	if m.isDirty() {
		return
	}
	if node.Path == m.currentFile && m.editorMode == modePreview {
		return
	}
	m.openFile(node.Path)
	// Show rendered markdown while browsing
	vp, err := newPreviewViewport(m.editor.Value(), m.editorWidth, m.editorHeight)
	if err == nil {
		m.preview = vp
		m.editorMode = modePreview
	}
}

func (m *model) startNewNote() {
	// Determine target directory from selected tree node
	dir := m.vault
	node := m.tree.selectedNode()
	if node != nil {
		if node.IsDir {
			dir = node.Path
		} else {
			dir = filepath.Dir(node.Path)
		}
	}

	m.newNoteDir = dir
	m.currentFile = ""
	m.editor.SetValue("")
	m.cleanContent = ""
	m.editor.Focus()
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	m.errMsg = ""
}

func (m *model) saveCurrentFile() {
	// New note: derive filename from first line
	if m.currentFile == "" && m.newNoteDir != "" {
		content := m.editor.Value()
		name := noteNameFromContent(content)
		if name == "" {
			m.errMsg = "Write something first — the first line becomes the filename"
			return
		}
		fullPath := filepath.Join(m.newNoteDir, name+".md")
		if _, err := os.Stat(fullPath); err == nil {
			m.errMsg = fmt.Sprintf("File already exists: %s", name+".md")
			return
		}
		if err := os.MkdirAll(m.newNoteDir, 0o755); err != nil {
			m.errMsg = fmt.Sprintf("Create dir failed: %v", err)
			return
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			m.errMsg = fmt.Sprintf("Save failed: %v", err)
			return
		}
		m.currentFile = fullPath
		m.newNoteDir = ""
		m.cleanContent = content
		m.errMsg = ""
		m.tree.currentFile = fullPath
		m.refreshTree()
		return
	}

	if m.currentFile == "" {
		m.errMsg = "No file open"
		return
	}
	content := m.editor.Value()
	if err := os.WriteFile(m.currentFile, []byte(content), 0o644); err != nil {
		m.errMsg = fmt.Sprintf("Save failed: %v", err)
		return
	}
	m.cleanContent = content
	m.errMsg = ""
	m.refreshTree()
}

func noteNameFromContent(content string) string {
	firstLine := strings.SplitN(content, "\n", 2)[0]
	// Strip markdown heading prefix
	firstLine = strings.TrimLeft(firstLine, "# ")
	firstLine = strings.TrimSpace(firstLine)
	// Sanitize: remove characters invalid in filenames
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "")
	firstLine = replacer.Replace(firstLine)
	if firstLine == "" {
		return ""
	}
	return firstLine
}



func (m *model) refreshTree() {
	root, err := buildTree(m.vault)
	if err != nil {
		m.errMsg = fmt.Sprintf("Refresh failed: %v", err)
		return
	}
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
		return
	}

	m.treeWidth = m.width / 4
	m.editorWidth = m.width - m.treeWidth
	m.treeHeight = m.height - 2
	m.editorHeight = m.height - 2

	m.tree.width = m.treeWidth
	m.tree.height = m.treeHeight

	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)

	if m.inputMode == inputPluginDiff {
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	}
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.width < 60 || m.height < 15 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			"Terminal too small\nMinimum: 60x15")
	}

	treeView := m.tree.View(m.activePanel == treePanel)

	if m.inputMode == inputFilter {
		treeView = m.filterView()
	}

	var rightView string
	if m.inputMode == inputPluginDiff {
		rightView = pluginDiffView(m.pluginDiffViewL, m.pluginDiffViewR, m.editorWidth, m.editorHeight)
	} else if m.currentFile == "" && m.newNoteDir == "" {
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

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, treeView, rightView)

	line, col := editorCursorPos(m.editor)
	filename := ""
	if m.newNoteDir != "" {
		filename = "[new note]"
	} else if m.currentFile != "" {
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
		dirty:      m.isDirty(),
		errMsg:     m.errMsg,
		fileOpen:   m.currentFile != "" || m.newNoteDir != "",
	}

	statusView := sb.View()
	if m.pluginProcessing {
		statusView = statusBarStyle.Width(m.width).Render("Processing...")
	} else if m.inputMode == inputPluginConfig {
		field := m.pluginConfigFields[m.pluginConfigIndex]
		statusView = statusBarStyle.Width(m.width).Render(
			field.Label + ": " + m.pluginConfigInput.View())
	} else if m.inputMode == inputPluginPrompt {
		statusView = statusBarStyle.Width(m.width).Render(
			"Prompt: " + m.pluginPromptInput.View())
	} else if m.inputMode == inputPluginDiff {
		statusView = statusBarStyle.Width(m.width).Render(
			"Accept changes? (y/n)")
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

	if m.inputMode == inputPluginSelect {
		statusView = pluginSelectorView(m.plugins, m.pluginCursor, m.width)
	}

	return lipgloss.JoinVertical(lipgloss.Left, mainView, statusView)
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
