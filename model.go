package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	chatPanelHit // used only for mouse hit-testing; never assigned to m.activePanel
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
	pendingDelete
)

type deleteCounts struct {
	files   int
	folders int
}

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
	inputShortcutSelect
	inputShortcutName
	inputShortcutDescription
	inputShortcutPrompt
	inputShortcutDeleteConfirm
	inputGitRemote
	inputRename
	inputHelp
	inputVaultSearch
	inputCapture
	inputDelegateName
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
	treeHidden bool // per-session toggle for Ctrl+B

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
	pendingDeletePath string // node path captured when Ctrl+D detours via inputUnsavedGuard

	deleteCount  deleteCounts // (files, folders) inside the folder being confirmed; zeroed for files
	deleteTarget string       // node.Path of the item awaiting confirmation; "" when not in inputConfirmDelete

	newFolderInput     textinput.Model
	renameInput        textinput.Model
	renameTarget       string
	renameIsDir        bool
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
	pluginCancel       context.CancelFunc
	activeChunks       <-chan string

	// AI shortcuts
	shortcuts              []AIShortcut
	shortcutCursor         int
	shortcutEditing        int
	shortcutTempName         string
	shortcutTempDescription  string
	aiRunOnSelection         bool
	shortcutPending          bool // true when shortcut awaits provider config completion
	shortcutNameInput        textinput.Model
	shortcutDescriptionInput textinput.Model
	shortcutPromptInput      textinput.Model
	activeShortcutProvider string // which AI provider runs shortcuts; cycled with 'p'

	// Git sync
	gitSyncRunning  bool
	gitSyncFlash    string
	gitSyncError    string
	gitSyncQuitting bool
	gitRemoteInput  textinput.Model

	// Help modal
	helpViewport viewport.Model

	// Vault index
	indexer       *Index
	indexerStatus string // status bar string; "" when idle

	// Vault search modal (Ctrl+Shift+F)
	vaultSearchInput   textinput.Model
	vaultSearchResults []searchResult
	vaultSearchCursor  int
	vaultSearchOffset  int
	vaultSearchPending bool
	vaultSearchToken   int64

	// Chat panel (Ctrl+Shift+A)
	chatOpen         bool
	chatWidth        int
	chatMode         chatModeT
	chatTurns        []chatTurn
	chatInput        textinput.Model
	chatViewport     viewport.Model
	chatStreaming    bool
	chatActiveChunks <-chan string
	chatCancel       context.CancelFunc
	chatCurrentCites []citation

	// Quick capture (Ctrl+J) and delegate-to-new-note (Ctrl+O)
	inboxPath     string         // raw config value; "" → default "inbox.md"
	captureInput  textarea.Model // multi-line, Shift+Enter for newline
	delegateInput textinput.Model
}

func newModel(vault string, plugins []Plugin, activeShortcutProvider, inboxPath string) model {
	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.CharLimit = 256

	pi := textinput.New()
	pi.Placeholder = "Enter prompt..."
	pi.CharLimit = 500

	nf := textinput.New()
	nf.Placeholder = "folder name"
	nf.CharLimit = 256

	rn := textinput.New()
	rn.Placeholder = "new name"
	rn.CharLimit = 256

	rs := textinput.New()
	rs.Placeholder = "search text"
	rs.CharLimit = 256

	rw := textinput.New()
	rw.Placeholder = "replace with"
	rw.CharLimit = 256

	sn := textinput.New()
	sn.Placeholder = "shortcut name"
	sn.CharLimit = 256

	sd := textinput.New()
	sd.Placeholder = "short description"
	sd.CharLimit = 120

	sp := textinput.New()
	sp.Placeholder = "prompt template"
	sp.CharLimit = 500

	gr := textinput.New()
	gr.Placeholder = "git@github.com:user/vault.git"
	gr.CharLimit = 512

	vsi := textinput.New()
	vsi.Placeholder = "Search note contents…"
	vsi.CharLimit = 256

	ci := textinput.New()
	ci.Placeholder = "Ask your vault…"
	ci.CharLimit = 1000

	cap := textarea.New()
	cap.Placeholder = "Quick capture (Enter saves, Shift+Enter for newline, Esc cancels)"
	cap.CharLimit = 0
	cap.SetWidth(56)
	cap.SetHeight(6)
	cap.ShowLineNumbers = false

	del := textinput.New()
	del.Placeholder = "filename (no .md needed)"
	del.CharLimit = 200
	del.Prompt = "Move to: "

	m := model{
		vault:                  vault,
		activePanel:            treePanel,
		editorMode:             modeEdit,
		editor:                 newSelectableEditor(),
		filterInput:            fi,
		newFolderInput:         nf,
		renameInput:            rn,
		replaceSearchInput:     rs,
		replaceWithInput:       rw,
		plugins:                plugins,
		pluginPromptInput:      pi,
		shortcutNameInput:        sn,
		shortcutDescriptionInput: sd,
		shortcutPromptInput:      sp,
		shortcutEditing:        -1,
		gitRemoteInput:         gr,
		activeShortcutProvider: activeShortcutProvider,
		vaultSearchInput:       vsi,
		chatInput:              ci,
		captureInput:           cap,
		delegateInput:          del,
		inboxPath:              inboxPath,
	}

	root, err := buildTree(vault)
	if err != nil {
		m.errMsg = fmt.Sprintf("Error reading vault: %v", err)
	} else {
		m.treeRoot = root
		m.tree.root = root
		m.tree.rebuildItems()
	}

	m.shortcuts, _ = loadShortcuts()

	return m
}

func (m model) isDirty() bool {
	return m.editor.Value() != m.cleanContent
}

// aiInputContent returns the content to feed to an AI run plus a flag the
// diff-accept path uses to decide whether to replace just the selection or
// the whole buffer. selActive is sufficient as the "has selection" predicate
// because the editor already clears it on no-op clicks and on cursor moves
// without shift, so a true value implies a non-empty range.
func (m *model) aiInputContent() (content string, onSelection bool) {
	if m.editor.selActive {
		return m.editor.SelectedText(), true
	}
	return m.editor.Value(), false
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, watchVault(m.vault), autoSaveTick(), gitSyncCheckImmediate()}
	if m.indexer != nil {
		cmds = append(cmds, startInitialIndex(m.indexer, m.vault))
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case fileChangedMsg:
		m.refreshTree()
		cmds := []tea.Cmd{watchVault(m.vault)}
		if m.indexer != nil {
			cmds = append(cmds, startInitialIndex(m.indexer, m.vault))
		}
		return m, tea.Batch(cmds...)

	case fileDeletedMsg:
		m.refreshTree()
		cmds := []tea.Cmd{watchVault(m.vault)}
		if m.indexer != nil {
			cmds = append(cmds, removeFileFromIndexCmd(m.indexer, msg.Path))
		}
		return m, tea.Batch(cmds...)

	case captureAppendedMsg:
		if msg.err != nil {
			m.errMsg = "capture failed: " + msg.err.Error()
			return m, nil
		}
		if msg.reloadOpen && m.currentFile == msg.inboxPath {
			if data, err := os.ReadFile(msg.inboxPath); err == nil {
				line, col := editorCursorPos(m.editor)
				m.editor.SetValue(string(data))
				m.cleanContent = string(data)
				m.editor.MoveTo(line, col)
			}
		}
		return m, nil

	case indexProgressMsg:
		if msg.embedded > 0 {
			m.indexerStatus = fmt.Sprintf("[idx %d/%d +%d]", msg.done, msg.total, msg.embedded)
		} else {
			m.indexerStatus = fmt.Sprintf("[idx %d/%d]", msg.done, msg.total)
		}
		// Chain: process the next file (or finish if this was the last).
		return m, processIndexFileCmd(msg.idx, msg.paths, msg.done, msg.embedded)

	case indexDoneMsg:
		if msg.err != nil {
			m.indexerStatus = "[idx error]"
			m.errMsg = "Index: " + msg.err.Error()
		} else if msg.embedded > 0 {
			m.indexerStatus = fmt.Sprintf("[idx done +%d]", msg.embedded)
		} else {
			m.indexerStatus = ""
		}
		return m, nil

	case indexFileMsg:
		return m, nil

	case vaultSearchResultsMsg:
		if msg.token != m.vaultSearchToken {
			return m, nil
		}
		m.vaultSearchPending = false
		if msg.err != nil {
			m.errMsg = "Search: " + msg.err.Error()
			return m, nil
		}
		m.vaultSearchResults = msg.results
		m.vaultSearchCursor = 0
		return m, nil

	case vaultSearchTickMsg:
		if msg.token != m.vaultSearchToken {
			return m, nil
		}
		if m.indexer == nil {
			return m, nil
		}
		m.vaultSearchPending = true
		return m, searchVaultCmd(m.indexer, msg.token, m.vaultSearchInput.Value(), 8, 80)

	case chatStartedMsg:
		m.chatActiveChunks = msg.chunks
		m.chatCurrentCites = msg.citations
		return m, streamChatCmd(msg.chunks, msg.errs)

	case chatStartFailedMsg:
		m.chatStreaming = false
		if len(m.chatTurns) > 0 && m.chatTurns[len(m.chatTurns)-1].Role == "assistant" {
			m.chatTurns = m.chatTurns[:len(m.chatTurns)-1]
		}
		m.errMsg = "Chat: " + msg.err.Error()
		return m, nil

	case chatChunkMsg:
		if msg.chunks != m.chatActiveChunks {
			return m, nil
		}
		last := &m.chatTurns[len(m.chatTurns)-1]
		last.Content += msg.delta
		innerW := m.chatWidth - 4
		if innerW < 1 {
			innerW = 1
		}
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
		m.chatViewport.GotoBottom()
		return m, readNextChatChunk(msg.chunks, msg.errs)

	case chatDoneMsg:
		if msg.chunks != m.chatActiveChunks {
			return m, nil
		}
		m.chatStreaming = false
		m.chatActiveChunks = nil
		m.chatCancel = nil
		last := &m.chatTurns[len(m.chatTurns)-1]
		last.Citations = m.chatCurrentCites
		m.chatCurrentCites = nil
		innerW := m.chatWidth - 4
		if innerW < 1 {
			innerW = 1
		}
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
		m.chatViewport.GotoBottom()
		return m, nil

	case chatErrMsg:
		if msg.chunks != m.chatActiveChunks {
			return m, nil
		}
		m.chatStreaming = false
		m.chatActiveChunks = nil
		last := &m.chatTurns[len(m.chatTurns)-1]
		last.Content = "Error: " + msg.err.Error()
		innerW := m.chatWidth - 4
		if innerW < 1 {
			innerW = 1
		}
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
		return m, nil

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

	case gitSyncCheckMsg:
		if m.gitSyncRunning {
			return m, gitSyncCheck()
		}
		cfg, err := loadConfig()
		if err != nil {
			return m, gitSyncCheck()
		}
		if cfg.GitRemote == "" {
			// No remote configured — prompt user
			m.inputMode = inputGitRemote
			m.gitRemoteInput.SetValue("")
			cmd := m.gitRemoteInput.Focus()
			return m, cmd
		}
		if cfg.LastSync != nil && time.Since(*cfg.LastSync) < 24*time.Hour {
			return m, gitSyncCheck()
		}
		m.gitSyncRunning = true
		m.gitSyncError = ""
		return m, tea.Batch(runGitSync(m.vault, cfg.GitRemote), gitSyncCheck())

	case gitSyncResultMsg:
		m.gitSyncRunning = false
		if msg.err != nil {
			m.gitSyncError = "Sync failed: " + msg.err.Error()
		} else if msg.pushErr != nil {
			m.gitSyncError = "Sync: push failed"
			// Still update LastSync since commit succeeded locally
			m.updateLastSync()
		} else {
			m.gitSyncError = ""
			m.updateLastSync()
			if msg.pulled && msg.pushed {
				m.gitSyncFlash = "Synced"
			} else if msg.pulled {
				m.gitSyncFlash = "Synced from remote"
				m.refreshTree()
			} else if msg.pushed {
				m.gitSyncFlash = "Backed up"
			}
		}
		if m.gitSyncFlash != "" {
			if m.gitSyncQuitting {
				return m, tea.Quit
			}
			return m, gitSyncFadeTick()
		}
		if m.gitSyncQuitting {
			return m, tea.Quit
		}
		return m, nil

	case gitSyncFadeMsg:
		m.gitSyncFlash = ""
		return m, nil

	case pluginChunkMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale: superseded or cancelled stream
		}
		m.pluginDiffResult += msg.delta
		halfWidth := m.editorWidth / 2
		rightWidth := m.editorWidth - halfWidth - 3
		if rightWidth < 1 {
			rightWidth = 1
		}
		m.pluginDiffViewR.SetContent(wordWrap(m.pluginDiffResult, rightWidth))
		m.pluginDiffViewR.GotoBottom()
		return m, readNextChunk(msg.chunks, msg.errs)

	case pluginDoneMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale
		}
		m.pluginProcessing = false
		m.pluginCancel = nil
		m.activeChunks = nil
		if m.pluginDiffResult == m.pluginDiffOriginal || m.pluginDiffResult == "" {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			m.pluginDiffResult = ""
		}
		return m, nil

	case pluginErrMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale
		}
		m.pluginProcessing = false
		m.pluginCancel = nil
		m.activeChunks = nil
		m.errMsg = "Plugin error: " + msg.err.Error()
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil

	case tea.MouseMsg:
		if m.pluginProcessing {
			return m, nil
		}
		if m.inputMode == inputHelp {
			return handleMouseMsg(m, msg)
		}
		if m.inputMode != inputNone {
			return m, nil
		}
		return handleMouseMsg(m, msg)

	case tea.KeyMsg:
		if m.pluginProcessing {
			switch msg.String() {
			case "esc":
				if m.pluginCancel != nil {
					m.pluginCancel()
					m.pluginCancel = nil
				}
				m.pluginProcessing = false
				m.activeChunks = nil
				m.inputMode = inputNone
				m.pluginActive = nil
				m.pluginDiffOriginal = ""
				m.pluginDiffResult = ""
				return m, nil
			case "ctrl+q":
				if m.isDirty() {
					m.inputMode = inputUnsavedGuard
					m.pendingAction = pendingQuit
					return m, nil
				}
				return m, tea.Quit
			}
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
			if m.gitSyncRunning {
				m.gitSyncQuitting = true
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
				cmd := m.editor.Paste()
				return m, cmd
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

		case "ctrl+b":
			m.treeHidden = !m.treeHidden
			if m.treeHidden && m.activePanel == treePanel {
				m.activePanel = editorPanel
				if m.currentFile != "" || m.newNoteDir != "" {
					cmd := m.editor.Focus()
					m.recalcLayout()
					return m, cmd
				}
			}
			m.recalcLayout()
			return m, nil

		case "f5":
			return m.triggerManualGitSync()

		case "ctrl+@":
			if m.currentFile != "" || m.newNoteDir != "" {
				if len(m.plugins) > 0 {
					m.inputMode = inputPluginSelect
					m.pluginCursor = 0
				}
			}
			return m, nil

		case "ctrl+g":
			if m.currentFile != "" || m.newNoteDir != "" {
				m.shortcuts, _ = loadShortcuts()
				m.inputMode = inputShortcutSelect
				m.shortcutCursor = 0
			}
			return m, nil

		case "ctrl+l":
			if m.currentFile != "" || m.newNoteDir != "" {
				m.shortcutEditing = -1
				m.inputMode = inputShortcutName
				m.shortcutNameInput.SetValue("")
				cmd := m.shortcutNameInput.Focus()
				return m, cmd
			}
			return m, nil

		case "ctrl+?", "ctrl+/", "ctrl+_":
			vp := viewport.New(m.editorWidth, m.editorHeight)
			vp.SetContent(helpContent(m.editorWidth))
			m.helpViewport = vp
			m.inputMode = inputHelp
			return m, nil

		case "ctrl+t":
			if m.indexer == nil || m.indexer.embedder == nil {
				m.errMsg = "Configure embedding_provider in config.toml"
				return m, nil
			}
			m.inputMode = inputVaultSearch
			m.vaultSearchInput.SetValue("")
			m.vaultSearchResults = nil
			m.vaultSearchCursor = 0
			m.vaultSearchOffset = 0
			cmd := m.vaultSearchInput.Focus()
			return m, cmd

		case "ctrl+k":
			if m.indexer == nil || m.indexer.embedder == nil {
				m.errMsg = "Configure embedding_provider in config.toml"
				return m, nil
			}
			if m.chatOpen {
				if m.chatCancel != nil {
					m.chatCancel()
					m.chatCancel = nil
				}
				m.chatOpen = false
				m.chatInput.Blur()
				m.recalcLayout()
				return m, nil
			}
			m.chatOpen = true
			m.chatMode = chatModeInput
			m.recalcLayout()
			cmd := m.chatInput.Focus()
			return m, cmd

		case "ctrl+j":
			if m.vault == "" {
				m.errMsg = "no vault configured"
				return m, nil
			}
			m.inputMode = inputCapture
			m.captureInput.Reset()
			cmd := m.captureInput.Focus()
			return m, cmd

		case "ctrl+o":
			if m.activePanel != editorPanel || m.currentFile == "" {
				m.errMsg = "open a file in the editor first"
				return m, nil
			}
			if !m.editor.selActive || m.editor.SelectedText() == "" {
				m.errMsg = "select text first"
				return m, nil
			}
			m.inputMode = inputDelegateName
			m.delegateInput.Reset()
			cmd := m.delegateInput.Focus()
			return m, cmd

		case "tab":
			if m.activePanel == treePanel {
				m.activePanel = editorPanel
				if m.editorMode == modeEdit {
					cmd := m.editor.Focus()
					return m, cmd
				}
				return m, nil
			}
			m.activePanel = treePanel
			m.editor.Blur()
			return m, nil
		}

		if m.chatOpen {
			return m.handleChatPanel(msg)
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
		if m.tree.onAddNote() {
			return m, nil
		}
		if m.currentFile != "" {
			m.activePanel = editorPanel
			m.editorMode = modeEdit
			cmd := m.editor.Focus()
			return m, cmd
		}
	case "enter":
		if m.tree.onAddNote() {
			if m.isDirty() {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingNewNote
				return m, nil
			}
			m.startNewNote()
			return m, nil
		}
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
		if node == nil {
			return m, nil
		}
		m.deleteTarget = node.Path
		m.deleteCount = deleteCounts{}
		if node.IsDir {
			files, folders, err := countTreeContents(node.Path)
			if err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
				m.deleteTarget = ""
				return m, nil
			}
			m.deleteCount = deleteCounts{files: files, folders: folders}
		}
		m.inputMode = inputConfirmDelete
		return m, nil
	case "ctrl+e":
		node := m.tree.selectedNode()
		if node != nil {
			m.renameTarget = node.Path
			m.renameIsDir = node.IsDir
			prefill := node.Name
			if !node.IsDir {
				prefill = strings.TrimSuffix(node.Name, filepath.Ext(node.Name))
			}
			m.renameInput.SetValue(prefill)
			m.renameInput.CursorEnd()
			m.inputMode = inputRename
			cmd := m.renameInput.Focus()
			return m, cmd
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
	if m.editorMode == modePreview {
		switch msg.String() {
		case "esc":
			m.activePanel = treePanel
			return m, nil
		case "enter", "right":
			m.editorMode = modeEdit
			cmd := m.editor.Focus()
			return m, cmd
		default:
			if msg.Type == tea.KeyRunes {
				m.editorMode = modeEdit
				focusCmd := m.editor.Focus()
				keyCmd := m.editor.HandleKey(msg)
				return m, tea.Batch(focusCmd, keyCmd)
			}
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}
	}

	// Edit mode
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
		if m.currentFile != "" {
			content := m.editor.Value()
			vp := viewport.New(m.editorWidth-2, m.editorHeight)
			vp.SetContent(wordWrap(content, m.editorWidth-4))
			m.preview = vp
			m.editorMode = modePreview
		}
		return m, nil
	}

	cmd := m.editor.HandleKey(msg)
	return m, cmd
}

func (m model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Guard ctrl+q across all input modes for graceful sync shutdown
	if msg.String() == "ctrl+q" && m.gitSyncRunning && !m.isDirty() {
		m.gitSyncQuitting = true
		return m, nil
	}
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
	case inputShortcutSelect:
		return m.handleShortcutSelect(msg)
	case inputShortcutName:
		return m.handleShortcutName(msg)
	case inputShortcutDescription:
		return m.handleShortcutDescription(msg)
	case inputShortcutPrompt:
		return m.handleShortcutPrompt(msg)
	case inputShortcutDeleteConfirm:
		return m.handleShortcutDeleteConfirm(msg)
	case inputGitRemote:
		return m.handleGitRemoteInput(msg)
	case inputRename:
		return m.handleRename(msg)
	case inputHelp:
		return m.handleHelp(msg)
	case inputVaultSearch:
		return m.handleVaultSearch(msg)
	case inputCapture:
		return m.handleCapture(msg)
	case inputDelegateName:
		return m.handleDelegate(msg)
	}
	return m, nil
}

func (m model) handleChatPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.chatStreaming {
		if msg.String() == "esc" {
			if m.chatCancel != nil {
				m.chatCancel()
				m.chatCancel = nil
			}
			m.chatStreaming = false
			m.chatActiveChunks = nil
			m.chatMode = chatModeView
			m.chatInput.Blur()
			return m, nil
		}
		return m, nil
	}

	switch m.chatMode {
	case chatModeInput:
		switch msg.String() {
		case "esc":
			m.chatMode = chatModeView
			m.chatInput.Blur()
			return m, nil
		case "enter":
			query := strings.TrimSpace(m.chatInput.Value())
			if query == "" {
				return m, nil
			}
			m.chatInput.SetValue("")
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "user", Content: query})
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant", Content: ""})
			m.chatStreaming = true

			provider := m.activeShortcutProvider
			if provider == "" {
				provider = defaultAIShortcutProvider
			}
			plugCfg, err := loadPluginConfig(provider)
			if err != nil {
				m.chatStreaming = false
				m.errMsg = "Plugin config: " + err.Error()
				if len(m.chatTurns) > 0 && m.chatTurns[len(m.chatTurns)-1].Role == "assistant" {
					m.chatTurns = m.chatTurns[:len(m.chatTurns)-1]
				}
				return m, nil
			}
			url := defaultBlackboxURL
			if provider == "openrouter" {
				url = defaultOpenRouterURL
			}
			ctx, cancel := context.WithCancel(context.Background())
			m.chatCancel = cancel
			_ = ctx

			// Render the user's question and the loading placeholder
			// immediately so they show before retrieval/streaming starts.
			innerW := m.chatWidth - 4
			if innerW < 1 {
				innerW = 1
			}
			m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
			m.chatViewport.GotoBottom()

			return m, chatStartCmd(m.indexer, m.chatTurns, query, url, plugCfg["api_key"], plugCfg["model"])
		}
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd
	case chatModeView:
		switch msg.String() {
		case "esc":
			m.chatOpen = false
			m.chatInput.Blur()
			m.recalcLayout()
			return m, nil
		case "i", "/":
			m.chatMode = chatModeInput
			cmd := m.chatInput.Focus()
			return m, cmd
		case "up", "k":
			m.chatViewport.LineUp(1)
			return m, nil
		case "down", "j":
			m.chatViewport.LineDown(1)
			return m, nil
		}
		s := msg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			n := int(s[0] - '0')
			cite := mostRecentCitation(m.chatTurns, n)
			if cite != nil {
				abs := filepath.Join(m.vault, cite.Path)
				if m.isDirty() {
					m.inputMode = inputUnsavedGuard
					m.pendingAction = pendingSwitchFile
					m.pendingSwitchPath = abs
					return m, nil
				}
				m.openFile(abs)
				m.editor.MoveTo(cite.StartLine-1, 0)
				m.activePanel = editorPanel
				m.editorMode = modeEdit
				return m, m.editor.Focus()
			}
		}
		return m, nil
	}
	return m, nil
}

func (m model) handleVaultSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputNone
		m.vaultSearchInput.Blur()
		return m, nil
	case "up":
		if m.vaultSearchCursor > 0 {
			m.vaultSearchCursor--
		}
		return m, nil
	case "down":
		if m.vaultSearchCursor < len(m.vaultSearchResults)-1 {
			m.vaultSearchCursor++
		}
		return m, nil
	case "enter":
		if len(m.vaultSearchResults) == 0 {
			return m, nil
		}
		r := m.vaultSearchResults[m.vaultSearchCursor]
		abs := filepath.Join(m.vault, r.Path)
		m.inputMode = inputNone
		m.vaultSearchInput.Blur()
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingSwitchFile
			m.pendingSwitchPath = abs
			return m, nil
		}
		m.openFile(abs)
		m.editor.MoveTo(r.StartLine-1, 0)
		m.activePanel = editorPanel
		m.editorMode = modeEdit
		return m, m.editor.Focus()
	}
	prev := m.vaultSearchInput.Value()
	var cmd tea.Cmd
	m.vaultSearchInput, cmd = m.vaultSearchInput.Update(msg)
	cur := m.vaultSearchInput.Value()
	if cur != prev {
		m.vaultSearchToken++
		return m, tea.Batch(cmd, vaultSearchTickCmd(m.vaultSearchToken))
	}
	return m, cmd
}

func (m model) handleHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+?", "ctrl+/", "ctrl+_", "q":
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
	m.helpViewport, cmd = m.helpViewport.Update(msg)
	return m, cmd
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
			var err error
			if node.IsDir {
				err = os.RemoveAll(node.Path)
			} else {
				err = os.Remove(node.Path)
			}
			if err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
			} else {
				if m.currentFile != "" && pathIsInside(m.currentFile, node.Path) {
					m.currentFile = ""
					m.editor.SetValue("")
					m.cleanContent = ""
					m.tree.currentFile = ""
				}
				m.refreshTree()
			}
		}
		m.inputMode = inputNone
		m.deleteTarget = ""
		m.deleteCount = deleteCounts{}
	case "n", "esc", "ctrl+c":
		m.inputMode = inputNone
		m.deleteTarget = ""
		m.deleteCount = deleteCounts{}
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

func (m model) handleRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.renameInput.Value())
		if name == "" {
			return m, nil
		}
		if err := m.doRename(name); err != nil {
			m.errMsg = err.Error()
			if strings.HasPrefix(err.Error(), "rename failed") {
				m.inputMode = inputNone
			}
			return m, nil
		}
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
	m.renameInput, cmd = m.renameInput.Update(msg)
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
		pre := m.editor.recordOp()
		newContent := strings.ReplaceAll(content, m.replaceSearchTerm, replacement)
		m.editor.SetValue(newContent)
		m.editor.commitOp(pre)
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
		if m.gitSyncRunning {
			m.gitSyncQuitting = true
			return m, nil
		}
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
	m.editor.ClearHistory()
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
	m.editor.ClearHistory()
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

	chatWidth := 0
	if m.chatOpen {
		chatWidth = m.width * 3 / 10
		if chatWidth < 40 {
			chatWidth = 40
		}
		if chatWidth > m.width/2 {
			chatWidth = m.width / 2
		}
	}

	// Hide tree when narrow terminal can't fit it, or when user has toggled
	// it off via Ctrl+B (per-session, not persisted).
	if m.treeHidden || m.width < minTreeWidth+10+chatWidth {
		m.treeWidth = 0
	} else {
		m.treeWidth = m.width / 4
		if m.treeWidth < minTreeWidth {
			m.treeWidth = minTreeWidth
		}
	}

	editorWidth := m.width - m.treeWidth - chatWidth
	if m.treeWidth > 0 {
		editorWidth-- // tree right border
	}
	if chatWidth > 0 {
		editorWidth-- // chat left border
	}
	if editorWidth < 10 {
		if m.treeWidth > 0 {
			m.treeWidth = 0
			editorWidth = m.width - chatWidth
			if chatWidth > 0 {
				editorWidth--
			}
		}
		if editorWidth < 10 && chatWidth > 0 {
			chatWidth = 0
			m.chatOpen = false
			editorWidth = m.width
		}
	}
	m.editorWidth = editorWidth
	m.chatWidth = chatWidth

	m.tree.width = m.treeWidth
	m.tree.height = m.treeHeight
	m.tree.clampOffset()

	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)

	if m.chatOpen {
		innerW := m.chatWidth - 4
		if innerW < 1 {
			innerW = 1
		}
		innerH := m.editorHeight - 4
		if innerH < 1 {
			innerH = 1
		}
		if m.chatViewport.Width == 0 {
			m.chatViewport = viewport.New(innerW, innerH)
		} else {
			m.chatViewport.Width = innerW
			m.chatViewport.Height = innerH
		}
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
		// Make the input fill the panel width so typed text doesn't clip
		// at the default textinput width.
		m.chatInput.Width = innerW - 2 // "> " prompt
		if m.chatInput.Width < 1 {
			m.chatInput.Width = 1
		}
	}

	if m.inputMode == inputPluginDiff {
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	}
	if m.inputMode == inputHelp {
		m.helpViewport.Width = m.editorWidth
		m.helpViewport.Height = m.editorHeight
		m.helpViewport.SetContent(helpContent(m.editorWidth))
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
	if m.inputMode == inputVaultSearch {
		modal := vaultSearchView(
			m.vaultSearchInput.View(),
			m.vaultSearchResults,
			m.vaultSearchCursor,
			m.vaultSearchOffset,
			m.editorWidth, m.editorHeight,
		)
		rightView = lipgloss.Place(m.editorWidth, m.editorHeight, lipgloss.Center, lipgloss.Center, modal)
	} else if m.inputMode == inputCapture {
		modal := captureView(
			m.captureInput.View(),
			resolveInboxPath(m.vault, m.inboxPath),
			m.editorWidth, m.editorHeight,
		)
		rightView = lipgloss.Place(m.editorWidth, m.editorHeight, lipgloss.Center, lipgloss.Center, modal)
	} else if m.inputMode == inputHelp {
		rightView = lipgloss.NewStyle().
			Width(m.editorWidth).
			MaxHeight(m.editorHeight).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1).
			Render(m.helpViewport.View())
	} else if m.inputMode == inputShortcutSelect {
		rightView = shortcutSelectorView(m.shortcuts, m.shortcutCursor, m.activeShortcutProvider, m.editorWidth, m.editorHeight)
	} else if m.inputMode == inputPluginDiff {
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
		style := previewStyle
		w := m.editorWidth
		if m.activePanel == editorPanel {
			style = previewFocusedStyle
			w-- // border adds 1 column outside Width
		}
		rightView = style.
			Width(w).
			Height(m.editorHeight).
			Render(m.preview.View())
	} else {
		rightView = editorStyle.
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(m.editor.View())
	}

	var columns []string
	if m.treeWidth > 0 {
		columns = append(columns, treeView)
	}
	columns = append(columns, rightView)
	if m.chatOpen && m.chatWidth > 0 {
		columns = append(columns, chatPanelView(m.chatViewport, m.chatInput.View(), m.chatMode, m.chatWidth, m.editorHeight))
	}
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, columns...)

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
		width:         m.width,
		treeActive:    m.activePanel == treePanel,
		filename:      filename,
		line:          line + 1,
		col:           col + 1,
		dirty:         m.isDirty(),
		errMsg:        m.errMsg,
		fileOpen:      m.currentFile != "" || m.newNoteDir != "",
		indexerStatus: m.indexerStatus,
	}
	if m.autoSaveFlash {
		sb.flashMsg = "Auto-saved"
	} else if m.gitSyncFlash != "" {
		sb.flashMsg = m.gitSyncFlash
	}
	if m.gitSyncError != "" {
		sb.errMsg = m.gitSyncError
	}

	statusView := sb.View()
	if m.gitSyncQuitting {
		statusView = statusBarStyle.Width(m.width).Render("Waiting for sync to finish...")
	} else if m.gitSyncRunning {
		statusView = statusBarStyle.Width(m.width).Render("Syncing...")
	} else if m.inputMode == inputGitRemote {
		statusView = statusBarStyle.Width(m.width).Render(
			"Git remote URL: " + m.gitRemoteInput.View())
	} else if m.pluginProcessing {
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
	} else if m.inputMode == inputRename {
		statusView = statusBarStyle.Width(m.width).Render(
			"Rename: " + m.renameInput.View())
	} else if m.inputMode == inputDelegateName {
		statusView = statusBarStyle.Width(m.width).Render(
			"Move to " + filepath.Dir(m.currentFile) + string(filepath.Separator) +
				m.delegateInput.View())
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
	} else if m.inputMode == inputShortcutName {
		statusView = statusBarStyle.Width(m.width).Render(
			"Shortcut name: " + m.shortcutNameInput.View())
	} else if m.inputMode == inputShortcutDescription {
		statusView = statusBarStyle.Width(m.width).Render(
			"Description: " + m.shortcutDescriptionInput.View())
	} else if m.inputMode == inputShortcutPrompt {
		statusView = statusBarStyle.Width(m.width).Render(
			"Prompt: " + m.shortcutPromptInput.View())
	} else if m.inputMode == inputShortcutDeleteConfirm {
		name := ""
		if m.shortcutCursor < len(m.shortcuts) {
			name = m.shortcuts[m.shortcutCursor].Name
		}
		statusView = statusBarStyle.Width(m.width).Render(
			fmt.Sprintf("Delete shortcut %q? (y/n)", name))
	} else if m.inputMode == inputConfirmDelete {
		node := m.tree.selectedNode()
		name := ""
		isDir := false
		if node != nil {
			name = node.Name
			isDir = node.IsDir
		}
		var prompt string
		switch {
		case !isDir:
			prompt = fmt.Sprintf("Delete %s? (y/n)", name)
		case m.deleteCount.files == 0 && m.deleteCount.folders == 0:
			prompt = fmt.Sprintf("Delete folder %q? (y/n)", name)
		default:
			prompt = fmt.Sprintf("Delete folder %q (%d files, %d folders)? (y/n)",
				name, m.deleteCount.files, m.deleteCount.folders)
		}
		statusView = statusBarStyle.Width(m.width).Render(prompt)
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
		runes := []rune(line)
		for lipgloss.Width(string(runes)) > width {
			// Find last space within visual width
			cut := -1
			w := 0
			for i, r := range runes {
				w += lipgloss.Width(string(r))
				if w > width {
					break
				}
				if r == ' ' {
					cut = i
				}
			}
			if cut <= 0 {
				// No space found — hard break at width
				w = 0
				for i, r := range runes {
					rw := lipgloss.Width(string(r))
					if w+rw > width {
						cut = i
						break
					}
					w += rw
				}
			}
			if cut <= 0 {
				break // safety: avoid infinite loop
			}
			result.WriteString(string(runes[:cut]))
			result.WriteByte('\n')
			runes = runes[cut:]
			// Trim leading spaces
			for len(runes) > 0 && runes[0] == ' ' {
				runes = runes[1:]
			}
		}
		result.WriteString(string(runes))
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

func (m *model) updateLastSync() {
	cfg, err := loadConfig()
	if err != nil {
		return
	}
	now := time.Now()
	cfg.LastSync = &now
	saveConfig(cfg)
}

func (m *model) doRename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name cannot contain path separators")
	}

	dir := filepath.Dir(m.renameTarget)
	var target string
	if m.renameIsDir {
		target = filepath.Join(dir, name)
	} else {
		target = filepath.Join(dir, name+filepath.Ext(m.renameTarget))
	}

	if target == m.renameTarget {
		return nil
	}

	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("already exists: %s", filepath.Base(target))
	}

	if err := os.Rename(m.renameTarget, target); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	if m.renameIsDir {
		prefix := m.renameTarget + string(os.PathSeparator)
		if strings.HasPrefix(m.currentFile, prefix) {
			rest := strings.TrimPrefix(m.currentFile, prefix)
			m.currentFile = filepath.Join(target, rest)
			m.tree.currentFile = m.currentFile
		}
		if m.fileClip.path == m.renameTarget || strings.HasPrefix(m.fileClip.path, prefix) {
			m.fileClip = fileClipboard{}
		}
		if m.tree.cutPath == m.renameTarget || strings.HasPrefix(m.tree.cutPath, prefix) {
			m.tree.cutPath = ""
		}
	} else {
		if m.currentFile == m.renameTarget {
			m.currentFile = target
			m.tree.currentFile = target
		}
		if m.fileClip.path == m.renameTarget {
			m.fileClip = fileClipboard{}
		}
		if m.tree.cutPath == m.renameTarget {
			m.tree.cutPath = ""
		}
	}

	return nil
}
