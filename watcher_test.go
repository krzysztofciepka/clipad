package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchVault_RemoveEmitsDeletedMsg(t *testing.T) {
	vault := t.TempDir()
	target := filepath.Join(vault, "x.md")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := watchVault(vault)

	type result struct{ msg interface{} }
	done := make(chan result, 1)
	go func() {
		done <- result{msg: cmd()}
	}()

	time.Sleep(100 * time.Millisecond)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-done:
		if d, ok := r.msg.(fileDeletedMsg); !ok {
			t.Errorf("got %T, want fileDeletedMsg", r.msg)
		} else if filepath.Base(d.Path) != "x.md" {
			t.Errorf("Path = %q, want trailing x.md", d.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not emit a message")
	}
}
