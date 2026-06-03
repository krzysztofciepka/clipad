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

type slashCommand int

const (
	slashNone    slashCommand = iota // not a slash command
	slashUnknown                     // slash prefix but unrecognised
	slashClear
	slashExit
	slashModel
	slashHelp
)

func (c slashCommand) String() string {
	switch c {
	case slashNone:
		return "slashNone"
	case slashUnknown:
		return "slashUnknown"
	case slashClear:
		return "slashClear"
	case slashExit:
		return "slashExit"
	case slashModel:
		return "slashModel"
	case slashHelp:
		return "slashHelp"
	default:
		return fmt.Sprintf("slashCommand(%d)", int(c))
	}
}

func parseSlashCommand(input string) slashCommand {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "/") {
		return slashNone
	}
	switch s {
	case "/clear":
		return slashClear
	case "/exit":
		return slashExit
	case "/model":
		return slashModel
	case "/help":
		return slashHelp
	default:
		return slashUnknown
	}
}

const agentHelpText = "Commands: /clear (reset conversation), /exit (close), /model (show model), /help (show this help). " +
	"Ask about your notes or tell me to manage files (rename, move, edit) in your vault."

func agentSystemPrompt(vault string) string {
	return fmt.Sprintf(`You are clipad's note-vault agent. You help the user manage and ask questions about their personal Markdown notes.

The notes vault is located at: %s
Your working directory is the vault root, and you are confined to it.

Tools:
- search_vault(query, k): semantic search over the notes. Use it to answer questions about note content. Cite excerpts by their numbered tag, e.g. [1].
- bash(cmd): run a shell command in the vault (ls, mv, cp, cat, sed, awk, etc.). Each call is a fresh, non-interactive shell, so directory changes do NOT persist between calls — chain steps in one command with && (e.g. cd subdir && cat file.md). Use it to inspect and modify files; inspect with read-only commands before making destructive changes.

Guidelines:
- For questions about the notes, prefer search_vault. For file management and content edits, use bash.
- Quote filenames that contain spaces.
- When you finish a task, end with a short plain-text summary of what you did.`, vault)
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
