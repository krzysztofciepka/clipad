package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
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

const indexSchema = `
CREATE TABLE IF NOT EXISTS chunks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	file_path   TEXT NOT NULL,
	start_line  INTEGER NOT NULL,
	end_line    INTEGER NOT NULL,
	text        TEXT NOT NULL,
	chunk_hash  TEXT NOT NULL,
	embedding   BLOB NOT NULL,
	model       TEXT NOT NULL,
	dim         INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_path);
CREATE INDEX IF NOT EXISTS idx_chunks_hash ON chunks(file_path, chunk_hash);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`

type Index struct {
	db       *sql.DB
	embedder EmbeddingClient
	vault    string // absolute vault root, used to relativize file_path
}

// indexDBPath returns the per-device SQLite path under XDG config.
func indexDBPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "index.db")
}

// OpenIndex opens (or creates) the SQLite index file and applies the schema.
// embedder may be nil; in that case Search/RebuildFile will fail with a clear
// error, but the DB still loads (so the model can show "configure provider").
func OpenIndex(path, vault string, embedder EmbeddingClient) (*Index, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(indexSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Index{db: db, embedder: embedder, vault: vault}, nil
}

func (idx *Index) Close() error { return idx.db.Close() }

// encodeEmbedding writes a float32 slice as little-endian bytes.
func encodeEmbedding(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

func decodeEmbedding(b []byte) []float32 {
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// cosine returns dot(a,b)/(|a||b|). Returns 0 if either is zero-norm.
func cosine(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / float32(math.Sqrt(float64(na)*float64(nb)))
}
