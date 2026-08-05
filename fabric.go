package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FabricPattern is one pattern discovered on disk. Only the metadata needed to
// render a picker row lives here; the prompt bodies are read by
// loadFabricPattern when a pattern actually runs, so opening the picker costs
// one readdir rather than hundreds of file reads.
type FabricPattern struct {
	Name        string
	Description string
}

// fabricPatternsDir returns the directory fabric keeps its patterns in.
// Clipad reads the files directly and never invokes the fabric CLI.
func fabricPatternsDir() string {
	if home := os.Getenv("FABRIC_CONFIG_HOME"); home != "" {
		return filepath.Join(home, "patterns")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "fabric", "patterns")
}

// explanationRe matches one entry of fabric's pattern_explanations.md, e.g.
// "12. **analyze_claims**: Analyse and rate truth claims."
var explanationRe = regexp.MustCompile(`^\s*\d+\.\s+\*\*([^*]+)\*\*:\s*(.+)$`)

// parsePatternExplanations extracts pattern name to one-line description.
// Lines that do not match the numbered-entry shape are skipped, so headings
// and prose in the file are ignored.
func parsePatternExplanations(data []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		m := explanationRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out[strings.TrimSpace(m[1])] = strings.TrimSpace(m[2])
	}
	return out
}

// listFabricPatterns returns every pattern in dir. A subdirectory qualifies
// when it holds a non-empty system.md. A missing or unreadable dir yields nil:
// clipad works fine without fabric installed, the picker just omits the
// section. os.ReadDir returns entries sorted by filename, so the result is
// already alphabetical.
func listFabricPatterns(dir string) []FabricPattern {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	descriptions := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(dir, "pattern_explanations.md")); err == nil {
		descriptions = parsePatternExplanations(data)
	}
	var patterns []FabricPattern
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, e.Name(), "system.md"))
		if err != nil || info.Size() == 0 {
			continue
		}
		patterns = append(patterns, FabricPattern{
			Name:        e.Name(),
			Description: descriptions[e.Name()],
		})
	}
	return patterns
}

// loadFabricPattern reads a pattern's prompt bodies. user is empty when the
// pattern has no user.md or it holds only whitespace — all but one of the
// stock patterns ship an empty user.md.
func loadFabricPattern(dir, name string) (system, user string, err error) {
	sys, err := os.ReadFile(filepath.Join(dir, name, "system.md"))
	if err != nil {
		return "", "", fmt.Errorf("reading fabric pattern %s: %w", name, err)
	}
	if usr, err := os.ReadFile(filepath.Join(dir, name, "user.md")); err == nil {
		if strings.TrimSpace(string(usr)) != "" {
			user = string(usr)
		}
	}
	return string(sys), user, nil
}

// fabricStreamURL resolves the provider endpoint. It is a variable so tests
// can point a pattern run at an httptest server.
var fabricStreamURL = shortcutProviderURL

// fabricUserMessage assembles the user message for a pattern run: the note
// body, prefixed by user.md when the pattern ships a non-empty one.
func fabricUserMessage(user, content string) string {
	if user == "" {
		return content
	}
	return user + "\n\n" + content
}

// runFabricPatternStream runs a fabric pattern through the AI-shortcut
// provider. Unlike runShortcutStream, which wraps a one-line instruction in
// clipad's own system prompt, a pattern's system.md *is* the system prompt —
// that is how fabric itself invokes them.
func runFabricPatternStream(ctx context.Context, system, user, content, provider string, config map[string]string) (<-chan string, <-chan error) {
	return streamChatCompletion(ctx, fabricStreamURL(provider),
		config["api_key"], config["model"], system, fabricUserMessage(user, content))
}
