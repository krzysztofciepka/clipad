package main

import (
	"strings"
	"unicode/utf8"
)

const readingWPM = 220

// stripCodeFences removes fenced code blocks delimited by lines whose trimmed
// text begins with "```" (three or more backticks), including the fence lines
// themselves. An unterminated opening fence strips to the end of the text.
// Only backtick fences are handled; tilde (~~~) fences are out of scope.
func stripCodeFences(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// computeMetrics strips code fences, then counts words (unicode-whitespace
// split, empty tokens ignored) and chars (runes of the stripped text,
// including spaces and newlines).
func computeMetrics(text string) (words, chars int) {
	stripped := stripCodeFences(text)
	words = len(strings.Fields(stripped))
	chars = utf8.RuneCountInString(stripped)
	return words, chars
}

// readingMinutes returns ceil(words / readingWPM). Yields 0 for words == 0 and
// >= 1 for any non-empty prose, so the "floor at 1m for non-empty" rule is
// automatic.
func readingMinutes(words int) int {
	if words <= 0 {
		return 0
	}
	return (words + readingWPM - 1) / readingWPM
}
