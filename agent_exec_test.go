package main

import "testing"

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
