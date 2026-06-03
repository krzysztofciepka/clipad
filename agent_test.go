package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDispatchTool_Bash(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, cites, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"echo hi"}`)
	if !ok {
		t.Errorf("ok = false, want true")
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("output %q missing 'hi'", out)
	}
	if cites != nil {
		t.Errorf("bash should produce no citations, got %v", cites)
	}
}

func TestDispatchTool_BashGuardRejects(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, _, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"sudo rm x"}`)
	if ok {
		t.Errorf("guarded command should report ok=false")
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("output %q should explain the block", out)
	}
}

func TestDispatchTool_SearchVaultNoEmbedder(t *testing.T) {
	deps := agentDeps{idx: nil}
	out, _, ok := dispatchTool(context.Background(), deps, "search_vault", `{"query":"taxes"}`)
	if ok {
		t.Errorf("search without embedder should report ok=false")
	}
	if !strings.Contains(strings.ToLower(out), "not configured") {
		t.Errorf("output %q should explain missing config", out)
	}
}

func TestDispatchTool_SearchVaultReturnsCitations(t *testing.T) {
	vault := t.TempDir()
	writeFile(t, vault, "a.md", "Taxes are due in April.\n\nUnrelated paragraph.")
	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.RebuildFile(context.Background(), vault+"/a.md"); err != nil {
		t.Fatal(err)
	}
	deps := agentDeps{idx: idx, vault: vault}
	out, cites, ok := dispatchTool(context.Background(), deps, "search_vault", `{"query":"Taxes are due in April.","k":2}`)
	if !ok {
		t.Fatalf("ok=false, out=%q", out)
	}
	if len(cites) == 0 {
		t.Errorf("expected citations, got none")
	}
	if !strings.Contains(out, "a.md") {
		t.Errorf("output %q should reference a.md", out)
	}
}

func TestDispatchTool_UnknownTool(t *testing.T) {
	out, _, ok := dispatchTool(context.Background(), agentDeps{}, "nope", `{}`)
	if ok || !strings.Contains(out, "unknown tool") {
		t.Errorf("unknown tool should report ok=false with message, got ok=%v out=%q", ok, out)
	}
}
