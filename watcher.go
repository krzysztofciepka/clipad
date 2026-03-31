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

func watchVault(vault string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}

		// Walk and watch all directories recursively
		addWatchDirs(watcher, vault)

		// Debounce: wait for events to settle before sending a message
		var debounce <-chan time.Time

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				// Skip dot-files/dirs
				base := filepath.Base(event.Name)
				if strings.HasPrefix(base, ".") {
					continue
				}
				// On create, watch new directories
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						addWatchDirs(watcher, event.Name)
					}
				}
				// Debounce: reset timer on each event
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
