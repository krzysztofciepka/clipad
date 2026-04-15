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
	"github.com/charmbracelet/x/ansi"
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
	inputNewFolder
	inputReplaceSearch
	inputReplaceWith
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

	editor       SelectableEditor
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

	newFolderInput    textinput.Model
	replaceSearchInput textinput.Model
	replaceWithInput   textinput.Model
	replaceSearchTerm  string

	errMsg string

	fileClip      fileClipboard
	autoSaveFlash bool

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

	nf := textinput.New()
	nf.Placeholder = "folder name"
	nf.CharLimit = 256

	rs := textinput.New()
	rs.Placeholder = "search text"
	rs.CharLimit = 256

	rw := textinput.New()
	rw.Placeholder = "replace with"
	rw.CharLimit = 256

	m := model{
		vault:              vault,
		activePanel:        treePanel,
		editorMode:         modeEdit,
		editor:             newSelectableEditor(),
		filterInput:        fi,
		newFolderInput:     nf,
		replaceSearchInput: rs,
		replaceWithInput:   rw,
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
	return tea.Batch(textarea.Blink, watchVault(m.vault), autoSaveTick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case fileChangedMsg:
		m.refreshTree()
		return m, watchVault(m.vault)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case autoSaveTickMsg:
		if m.currentFile != "" && m.isDirty() {
			m.saveCurrentFile()
			if m.errMsg == "" {
				m.autoSaveFlash = true
				return m, tea.Batch(autoSaveTick(), autoSaveFadeTick())
			}
		}
		return m, autoSaveTick()

	case autoSaveFadeMsg:
		m.autoSaveFlash = false
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
		case "ctrl+q":
			if m.isDirty() {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingQuit
				return m, nil
			}
			return m, tea.Quit

		case "ctrl+c":
			if m.activePanel == treePanel {
				node := m.tree.selectedNode()
				if node != nil && !node.IsDir {
					m.fileClip = fileClipboard{path: node.Path, op: clipCopy}
					m.tree.cutPath = ""
					m.errMsg = "Copied: " + node.Name
				}
				return m, nil
			}
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				m.editor.Copy()
			}
			return m, nil

		case "ctrl+x":
			if m.activePanel == treePanel {
				node := m.tree.selectedNode()
				if node != nil && !node.IsDir {
					m.fileClip = fileClipboard{path: node.Path, op: clipCut}
					m.tree.cutPath = node.Path
					m.errMsg = "Cut: " + node.Name
				}
				return m, nil
			}
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				m.editor.Cut()
			}
			return m, nil

		case "ctrl+v":
			if m.activePanel == treePanel {
				if !m.fileClip.empty() {
					m.pasteFile()
				}
				return m, nil
			}
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				m.editor.Paste()
			}
			return m, nil

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

		case "ctrl+r":
			if m.currentFile != "" || m.newNoteDir != "" {
				m.inputMode = inputReplaceSearch
				m.replaceSearchInput.SetValue("")
				m.replaceSearchTerm = ""
				cmd := m.replaceSearchInput.Focus()
				return m, cmd
			}
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
		m.editor.Model, cmd = m.editor.Model.Update(msg)
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
	case "right":
		if m.currentFile != "" {
			m.activePanel = editorPanel
			m.editorMode = modeEdit
			cmd := m.editor.Focus()
			return m, cmd
		}
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
			m.activePanel = editorPanel
			m.editorMode = modeEdit
			cmd := m.editor.Focus()
			return m, cmd
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
	case "ctrl+f":
		m.inputMode = inputNewFolder
		m.newFolderInput.SetValue("")
		cmd := m.newFolderInput.Focus()
		return m, cmd
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
	if msg.String() == "esc" {
		m.editor.ClearSelection()
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingSwitchFile
			m.pendingSwitchPath = m.currentFile
			return m, nil
		}
		m.activePanel = treePanel
		m.editor.Blur()
		// Switch to preview mode so the note shows as read-only
		if m.currentFile != "" {
			content := m.editor.Value()
			vp := viewport.New(m.editorWidth-2, m.editorHeight)
			vp.SetContent(wordWrap(content, m.editorWidth-4))
			m.preview = vp
			m.editorMode = modePreview
		}
		return m, nil
	}

	if m.editorMode == modePreview {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}

	cmd := m.editor.HandleKey(msg)
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
	case inputNewFolder:
		return m.handleNewFolder(msg)
	case inputReplaceSearch:
		return m.handleReplaceSearch(msg)
	case inputReplaceWith:
		return m.handleReplaceWith(msg)
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

func (m model) handleNewFolder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := m.newFolderInput.Value()
		if name == "" {
			return m, nil
		}
		// Determine parent dir from selected tree node
		dir := m.vault
		node := m.tree.selectedNode()
		if node != nil {
			if node.IsDir {
				dir = node.Path
			} else {
				dir = filepath.Dir(node.Path)
			}
		}
		folderPath := filepath.Join(dir, name)
		if err := os.MkdirAll(folderPath, 0o755); err != nil {
			m.errMsg = fmt.Sprintf("Create folder failed: %v", err)
			m.inputMode = inputNone
			return m, nil
		}
		// Create a placeholder note so the folder shows in the tree
		os.WriteFile(filepath.Join(folderPath, "untitled.md"), []byte(""), 0o644)
		m.refreshTree()
		m.inputMode = inputNone
		m.errMsg = ""
		return m, nil
	case "esc":
		m.inputMode = inputNone
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.newFolderInput, cmd = m.newFolderInput.Update(msg)
	return m, cmd
}

func (m model) handleReplaceSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		term := m.replaceSearchInput.Value()
		if term == "" {
			return m, nil
		}
		// Check if the term exists in the content
		content := m.editor.Value()
		count := strings.Count(content, term)
		if count == 0 {
			m.errMsg = "No matches found"
			m.inputMode = inputNone
			m.editorMode = modeEdit
			return m, nil
		}
		m.replaceSearchTerm = term
		m.inputMode = inputReplaceWith
		m.replaceWithInput.SetValue("")
		cmd := m.replaceWithInput.Focus()
		m.errMsg = fmt.Sprintf("%d match(es) found", count)
		return m, cmd
	case "esc":
		m.inputMode = inputNone
		m.editorMode = modeEdit
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.replaceSearchInput, cmd = m.replaceSearchInput.Update(msg)
	return m, cmd
}

func (m model) handleReplaceWith(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		replacement := m.replaceWithInput.Value()
		content := m.editor.Value()
		newContent := strings.ReplaceAll(content, m.replaceSearchTerm, replacement)
		m.editor.SetValue(newContent)
		count := strings.Count(content, m.replaceSearchTerm)
		m.errMsg = fmt.Sprintf("Replaced %d occurrence(s)", count)
		m.inputMode = inputNone
		m.editorMode = modeEdit
		m.replaceSearchTerm = ""
		return m, nil
	case "esc":
		m.inputMode = inputNone
		m.editorMode = modeEdit
		m.replaceSearchTerm = ""
		m.errMsg = ""
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.replaceWithInput, cmd = m.replaceWithInput.Update(msg)
	return m, cmd
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
	if node.Path == m.currentFile {
		return
	}
	m.openFile(node.Path)
	// Show raw text in read-only viewport so keystrokes don't leak into editor.
	// Markdown rendering only happens on explicit Ctrl+P.
	content := m.editor.Value()
	vp := viewport.New(m.editorWidth-2, m.editorHeight)
	vp.SetContent(wordWrap(content, m.editorWidth-4))
	m.preview = vp
	m.editorMode = modePreview
	m.editor.Blur()
	m.activePanel = treePanel
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



func (m *model) pasteFile() {
	src := m.fileClip.path
	if _, err := os.Stat(src); err != nil {
		m.errMsg = "Source file not found"
		m.fileClip = fileClipboard{}
		m.tree.cutPath = ""
		return
	}

	dir := m.vault
	node := m.tree.selectedNode()
	if node != nil {
		if node.IsDir {
			dir = node.Path
		} else {
			dir = filepath.Dir(node.Path)
		}
	}

	dst := uniquePath(filepath.Join(dir, filepath.Base(src)))

	if m.fileClip.op == clipCut {
		if err := os.Rename(src, dst); err != nil {
			m.errMsg = fmt.Sprintf("Move failed: %v", err)
			return
		}
		if m.currentFile == src {
			m.currentFile = dst
			m.tree.currentFile = dst
		}
	} else {
		if err := copyFile(src, dst); err != nil {
			m.errMsg = fmt.Sprintf("Copy failed: %v", err)
			return
		}
	}

	m.fileClip = fileClipboard{}
	m.tree.cutPath = ""
	m.errMsg = ""
	m.refreshTree()
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
	m.treeHeight = m.height - 2
	if m.treeHeight < 1 {
		m.treeHeight = 1
	}
	m.editorHeight = m.height - 2
	if m.editorHeight < 1 {
		m.editorHeight = 1
	}

	const minTreeWidth = 20

	// Hide tree only on extremely narrow terminals where it can't fit
	if m.width < minTreeWidth+10 {
		m.treeWidth = 0
		m.editorWidth = m.width
	} else {
		m.treeWidth = m.width / 4
		if m.treeWidth < minTreeWidth {
			m.treeWidth = minTreeWidth
		}
		m.editorWidth = m.width - m.treeWidth - 1 // -1 for tree panel's right border
		if m.editorWidth < 10 {
			m.editorWidth = 10
			m.treeWidth = m.width - m.editorWidth - 1
		}
	}

	m.tree.width = m.treeWidth
	m.tree.height = m.treeHeight
	m.tree.clampOffset()

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



	var treeView string
	if m.treeWidth > 0 {
		treeView = m.tree.View(m.activePanel == treePanel)
		if m.inputMode == inputFilter {
			treeView = m.filterView()
		}
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
	} else if m.inputMode == inputReplaceSearch || m.inputMode == inputReplaceWith {
		// Show content with highlighted matches
		content := m.editor.Value()
		term := m.replaceSearchInput.Value()
		if m.inputMode == inputReplaceWith {
			term = m.replaceSearchTerm
		}
		highlighted := highlightMatches(content, term, m.editorWidth-4)
		vp := viewport.New(m.editorWidth-2, m.editorHeight)
		vp.SetContent(highlighted)
		rightView = previewStyle.
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(vp.View())
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

	var mainView string
	if m.treeWidth > 0 {
		mainView = lipgloss.JoinHorizontal(lipgloss.Top, treeView, rightView)
	} else {
		mainView = rightView
	}

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
	if m.autoSaveFlash {
		sb.flashMsg = "Auto-saved"
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
	} else if m.inputMode == inputNewFolder {
		statusView = statusBarStyle.Width(m.width).Render(
			"New folder: " + m.newFolderInput.View())
	} else if m.inputMode == inputReplaceSearch {
		term := m.replaceSearchInput.Value()
		countInfo := ""
		if term != "" {
			count := strings.Count(m.editor.Value(), term)
			countInfo = fmt.Sprintf("  (%d found)", count)
		}
		statusView = statusBarStyle.Width(m.width).Render(
			"Find: " + m.replaceSearchInput.View() + countInfo)
	} else if m.inputMode == inputReplaceWith {
		statusView = statusBarStyle.Width(m.width).Render(
			"Replace with: " + m.replaceWithInput.View())
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

	output := lipgloss.JoinVertical(lipgloss.Left, mainView, statusView)

	// Ensure output never exceeds terminal dimensions to prevent scrolling
	lines := strings.Split(output, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i := range lines {
		if lipgloss.Width(lines[i]) > m.width {
			lines[i] = ansi.Truncate(lines[i], m.width, "")
		}
	}
	return strings.Join(lines, "\n")
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

	maxW := m.treeWidth - 2 // content area for treePanelStyle padding
	for i := start; i < end; i++ {
		line := m.filterResults[i].Name
		if maxW > 0 && lipgloss.Width(line) > maxW {
			line = ansi.Truncate(line, maxW, "…")
		}
		if i == m.filterCursor {
			line = treeSelectedStyle.Render(line)
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return treePanelStyle.Width(m.treeWidth).MaxHeight(m.treeHeight).Render(b.String())
}

func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var result strings.Builder
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			// Find last space within width
			cut := strings.LastIndex(line[:width], " ")
			if cut <= 0 {
				cut = width
			}
			result.WriteString(line[:cut])
			result.WriteByte('\n')
			line = line[cut:]
			line = strings.TrimLeft(line, " ")
		}
		result.WriteString(line)
		result.WriteByte('\n')
	}
	return strings.TrimRight(result.String(), "\n")
}

var highlightStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("226")).
	Foreground(lipgloss.Color("0")).
	Bold(true)

func highlightMatches(content, term string, wrapWidth int) string {
	wrapped := wordWrap(content, wrapWidth)
	if term == "" {
		return wrapped
	}
	var result strings.Builder
	remaining := wrapped
	for {
		idx := strings.Index(remaining, term)
		if idx < 0 {
			result.WriteString(remaining)
			break
		}
		result.WriteString(remaining[:idx])
		result.WriteString(highlightStyle.Render(term))
		remaining = remaining[idx+len(term):]
	}
	return result.String()
}
