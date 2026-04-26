package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

type fileChangedMsg struct{}
type fileDeletedMsg struct{ Path string }

func watchVault(vault string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}
		addWatchDirs(watcher, vault)
		var debounce <-chan time.Time
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				base := filepath.Base(event.Name)
				if strings.HasPrefix(base, ".") {
					continue
				}
				if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
					return fileDeletedMsg{Path: event.Name}
				}
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						addWatchDirs(watcher, event.Name)
					}
				}
				debounce = time.After(100 * time.Millisecond)
			case <-debounce:
				return fileChangedMsg{}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}

func addWatchDirs(watcher *fsnotify.Watcher, root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			watcher.Add(path)
		}
		return nil
	})
}
