package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxToolCalls  = 25 // hard cap on tool calls per user message
	maxToolOutput = 4096
	bashTimeout   = 30 * time.Second // per-command wall-clock timeout
)

var sudoRE = regexp.MustCompile(`(^|\s)sudo(\s|$)`)

// vaultGuard returns a non-nil error if cmd should not run because it could
// escape the vault or is disallowed. This is a best-effort guard against
// accidents — cwd=vault is the real scoping mechanism, and a determined
// command could still escape. It rejects: sudo, "~" references, absolute
// paths not under the vault, and ".." traversal that escapes the vault root.
func vaultGuard(vault, cmd string) error {
	if sudoRE.MatchString(cmd) {
		return fmt.Errorf("blocked: sudo is not allowed")
	}
	for _, tok := range strings.Fields(cmd) {
		t := strings.Trim(tok, "'\"")
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "~") {
			return fmt.Errorf("blocked: home (~) reference %q escapes the vault", t)
		}
		var abs string
		if filepath.IsAbs(t) {
			abs = filepath.Clean(t)
		} else if strings.Contains(t, "..") {
			abs = filepath.Clean(filepath.Join(vault, t))
		} else {
			continue
		}
		if !underDir(vault, abs) {
			return fmt.Errorf("blocked: path %q is outside the vault", t)
		}
	}
	return nil
}

// underDir reports whether path is the base dir or inside it.
func underDir(base, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// runBashInVault runs `bash -c cmd` with cwd=vault, no stdin, a wall-clock
// timeout, and combined stdout+stderr truncated to maxToolOutput bytes.
// Returns the (possibly truncated) output and the process exit code. err is
// non-nil only for failures to start/execute the process itself — a non-zero
// exit or a timeout is reported via the exit code, not err.
func runBashInVault(ctx context.Context, vault, cmd string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(ctx, "bash", "-c", cmd)
	c.Dir = vault
	c.Stdin = nil // no stdin; reads return EOF immediately
	c.WaitDelay = timeout + 2*time.Second

	out, err := c.CombinedOutput()
	output := truncateBytes(out, maxToolOutput)

	if ctx.Err() == context.DeadlineExceeded {
		return output + "\n[timed out after " + timeout.String() + "]", 124, nil
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return output, ee.ExitCode(), nil
		}
		return output, -1, fmt.Errorf("run: %w", err)
	}
	return output, 0, nil
}

// truncateBytes returns b as a string, truncated to max bytes with a marker.
func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "\n[output truncated]"
}
