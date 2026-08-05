package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

type clipOp int

const (
	clipCut clipOp = iota
	clipCopy
)

type fileClipboard struct {
	path string
	op   clipOp
}

func (c fileClipboard) empty() bool {
	return c.path == ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// writeClipboard writes text to the system clipboard. Indirected through a
// variable so tests can observe copies without clobbering the real clipboard.
var writeClipboard = clipboard.WriteAll

// copyFlashMsg clears the "Copied" status-bar confirmation.
type copyFlashMsg struct{}

// copyFlashTick fades the copy confirmation after the same delay as the
// auto-save and git-sync flashes.
func copyFlashTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return copyFlashMsg{}
	})
}

// flashCopied shows the green "Copied" confirmation in the status bar and
// returns the command that clears it. Clears any lingering m.errMsg: the
// status bar renders errMsg instead of the flash while errMsg is non-empty,
// and errMsg is sticky (only cleared on file open/save), so without this a
// stale message like "select text first" would permanently suppress the
// flash. This mirrors the rest of the codebase's convention of clearing
// errMsg on a fresh user action.
func (m *model) flashCopied() tea.Cmd {
	m.errMsg = ""
	m.copyFlash = true
	return copyFlashTick()
}

// flashText resolves which non-error flash the status bar shows. The copy
// confirmation wins: it acknowledges an explicit user action, where auto-save
// and git-sync are background events the user did not just trigger.
func flashText(copyFlash, autoSaveFlash bool, gitSyncFlash string) string {
	switch {
	case copyFlash:
		return "Copied"
	case autoSaveFlash:
		return "Auto-saved"
	default:
		return gitSyncFlash
	}
}
