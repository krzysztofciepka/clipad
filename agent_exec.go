package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
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
