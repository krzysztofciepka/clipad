package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const maxChunkChars = 2000

type chunk struct {
	StartLine int
	EndLine   int
	Text      string
	Hash      string
}

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

// chunkFile splits a markdown file into chunks. Paragraphs (separated by one
// or more blank lines) are the unit; paragraphs longer than maxChunkChars are
// further split on line boundaries until each sub-chunk fits.
//
// Lines are 1-indexed; start_line/end_line are inclusive.
func chunkFile(text string) []chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	var chunks []chunk
	i := 0
	for i < len(lines) {
		// Skip blank lines.
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) {
			break
		}
		startLine := i + 1
		paraLines := []string{}
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			paraLines = append(paraLines, lines[i])
			i++
		}
		endLine := startLine + len(paraLines) - 1

		paragraph := strings.Join(paraLines, "\n")
		if len(paragraph) <= maxChunkChars {
			c := chunk{StartLine: startLine, EndLine: endLine, Text: paragraph}
			c.Hash = chunkHash(c.Text)
			chunks = append(chunks, c)
			continue
		}
		// Oversize: split by lines, accumulating until adding the next line would exceed cap.
		subStart := startLine
		var buf []string
		bufLen := 0
		for _, l := range paraLines {
			lineLen := len(l) + 1 // include separator newline
			if bufLen > 0 && bufLen+lineLen > maxChunkChars {
				text := strings.Join(buf, "\n")
				c := chunk{StartLine: subStart, EndLine: subStart + len(buf) - 1, Text: text}
				c.Hash = chunkHash(c.Text)
				chunks = append(chunks, c)
				subStart += len(buf)
				buf = nil
				bufLen = 0
			}
			buf = append(buf, l)
			bufLen += lineLen
		}
		if len(buf) > 0 {
			text := strings.Join(buf, "\n")
			c := chunk{StartLine: subStart, EndLine: subStart + len(buf) - 1, Text: text}
			c.Hash = chunkHash(c.Text)
			chunks = append(chunks, c)
		}
	}
	return chunks
}
