package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// agentDeps bundles everything a tool call or the loop needs.
type agentDeps struct {
	url     string
	apiKey  string
	model   string
	vault   string
	idx     *Index
	timeout time.Duration
}

// dispatchTool executes a single tool call and returns the tool-result text
// (always non-empty, fed back to the model), any citations (search_vault only),
// and whether the tool succeeded. A failure (ok=false) still returns a message
// describing why so the model can recover.
func dispatchTool(ctx context.Context, deps agentDeps, name, argsJSON string) (string, []citation, bool) {
	switch name {
	case "bash":
		cmd, err := parseBashArgs(argsJSON)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		if err := vaultGuard(deps.vault, cmd); err != nil {
			return err.Error(), nil, false
		}
		timeout := deps.timeout
		if timeout <= 0 {
			timeout = bashTimeout
		}
		out, code, err := runBashInVault(ctx, deps.vault, cmd, timeout)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		return fmt.Sprintf("exit %d\n%s", code, out), nil, code == 0

	case "search_vault":
		query, k, err := parseSearchArgs(argsJSON)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		if deps.idx == nil || !deps.idx.IsSearchable() {
			return "semantic search is not configured (set embedding_provider in config.toml)", nil, false
		}
		_, _ = deps.idx.PruneOrphans(ctx)
		results, err := deps.idx.Search(ctx, query, k)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		return formatSearchResults(results)

	default:
		return "unknown tool: " + name, nil, false
	}
}

func parseBashArgs(argsJSON string) (string, error) {
	var a struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("bad bash arguments: %w", err)
	}
	if strings.TrimSpace(a.Cmd) == "" {
		return "", fmt.Errorf("bash: empty cmd")
	}
	return a.Cmd, nil
}

func parseSearchArgs(argsJSON string) (string, int, error) {
	var a struct {
		Query string `json:"query"`
		K     int    `json:"k"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", 0, fmt.Errorf("bad search arguments: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", 0, fmt.Errorf("search_vault: empty query")
	}
	if a.K <= 0 {
		a.K = 8
	}
	return a.Query, a.K, nil
}

// formatSearchResults renders hits as numbered excerpts and extracts citations.
func formatSearchResults(results []Result) (string, []citation, bool) {
	if len(results) == 0 {
		return "No matching notes found.", nil, true
	}
	var b strings.Builder
	cites := make([]citation, len(results))
	for i, r := range results {
		fmt.Fprintf(&b, "[%d] %s L%d-L%d:\n%s\n\n", i+1, r.Path, r.StartLine, r.EndLine, r.Text)
		cites[i] = citation{Path: r.Path, StartLine: r.StartLine, EndLine: r.EndLine}
	}
	return strings.TrimSpace(b.String()), cites, true
}
