package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestVaultGuard_AllowsInVaultCommands(t *testing.T) {
	vault := "/home/u/vault"
	ok := []string{
		"ls",
		"mv 'Task 9.md' 'Task 1.md'",
		"cat ./notes/x.md",
		"sed -i 's/a/b/' notes/x.md",
		"cat /home/u/vault/a.md", // absolute, but inside vault
	}
	for _, c := range ok {
		if err := vaultGuard(vault, c); err != nil {
			t.Errorf("vaultGuard(%q) = %v, want nil", c, err)
		}
	}
}

func TestVaultGuard_RejectsEscapes(t *testing.T) {
	vault := "/home/u/vault"
	bad := []string{
		"sudo rm -rf x",
		"cat /etc/passwd",
		"cat ../../secret",
		"mv a.md ../outside.md",
		"cat ~/secrets",
	}
	for _, c := range bad {
		if err := vaultGuard(vault, c); err == nil {
			t.Errorf("vaultGuard(%q) = nil, want error", c)
		}
	}
}

func TestRunBashInVault_RunsInVaultCwd(t *testing.T) {
	vault := t.TempDir()
	out, code, err := runBashInVault(context.Background(), vault, "pwd", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out) != vault {
		t.Errorf("pwd = %q, want %q", strings.TrimSpace(out), vault)
	}
}

func TestRunBashInVault_CapturesStderrAndExitCode(t *testing.T) {
	vault := t.TempDir()
	out, code, err := runBashInVault(context.Background(), vault, "echo oops >&2; exit 3", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(out, "oops") {
		t.Errorf("output %q missing stderr", out)
	}
}

func TestRunBashInVault_TimesOut(t *testing.T) {
	vault := t.TempDir()
	_, code, err := runBashInVault(context.Background(), vault, "sleep 5", 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if code != 124 {
		t.Errorf("timed-out command: exit code = %d, want 124", code)
	}
}

func TestRunBashInVault_TruncatesOutput(t *testing.T) {
	vault := t.TempDir()
	out, _, err := runBashInVault(context.Background(), vault, "yes x | head -c 100000", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > maxToolOutput+64 {
		t.Errorf("output not truncated: %d bytes", len(out))
	}
}
