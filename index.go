package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// relPath returns the path relative to the index's vault root.
func (idx *Index) relPath(absPath string) (string, error) {
	rel, err := filepath.Rel(idx.vault, absPath)
	if err != nil {
		return "", fmt.Errorf("relpath: %w", err)
	}
	return rel, nil
}

// RebuildFile re-chunks the file at absPath and updates the index so that
// chunks(file_path = rel) exactly matches the new chunk set, embedding only
// chunks whose hash isn't already present (model-matched).
func (idx *Index) RebuildFile(ctx context.Context, absPath string) error {
	if idx.embedder == nil {
		return errors.New("index: no embedder configured")
	}
	rel, err := idx.relPath(absPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	newChunks := chunkFile(string(data))

	model := idx.embedder.Model()
	existing := map[string]int64{}
	rows, err := idx.db.QueryContext(ctx,
		`SELECT id, chunk_hash FROM chunks WHERE file_path = ? AND model = ?`, rel, model)
	if err != nil {
		return fmt.Errorf("select existing: %w", err)
	}
	for rows.Next() {
		var id int64
		var h string
		if err := rows.Scan(&id, &h); err != nil {
			rows.Close()
			return err
		}
		existing[h] = id
	}
	rows.Close()

	keep := map[int64]bool{}
	var toEmbed []chunk
	for _, c := range newChunks {
		if id, ok := existing[c.Hash]; ok {
			keep[id] = true
		} else {
			toEmbed = append(toEmbed, c)
		}
	}

	tx, err := idx.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	delRows, err := tx.QueryContext(ctx,
		`SELECT id FROM chunks WHERE file_path = ? AND model = ?`, rel, model)
	if err != nil {
		return fmt.Errorf("select for delete: %w", err)
	}
	var idsToDelete []int64
	for delRows.Next() {
		var id int64
		if err := delRows.Scan(&id); err != nil {
			delRows.Close()
			return err
		}
		if !keep[id] {
			idsToDelete = append(idsToDelete, id)
		}
	}
	delRows.Close()
	for _, id := range idsToDelete {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
	}

	if len(toEmbed) > 0 {
		texts := make([]string, len(toEmbed))
		for i, c := range toEmbed {
			texts[i] = c.Text
		}
		vecs, err := idx.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}
		if len(vecs) != len(toEmbed) {
			return fmt.Errorf("embed: got %d vectors for %d chunks", len(vecs), len(toEmbed))
		}
		now := time.Now().Unix()
		for i, c := range toEmbed {
			blob := encodeEmbedding(vecs[i])
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO chunks(file_path, start_line, end_line, text, chunk_hash, embedding, model, dim, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				rel, c.StartLine, c.EndLine, c.Text, c.Hash, blob, model, len(vecs[i]), now); err != nil {
				return fmt.Errorf("insert: %w", err)
			}
		}
	}

	return tx.Commit()
}

// RemoveFile deletes all chunks for the file at absPath.
func (idx *Index) RemoveFile(ctx context.Context, absPath string) error {
	rel, err := idx.relPath(absPath)
	if err != nil {
		return err
	}
	_, err = idx.db.ExecContext(ctx, `DELETE FROM chunks WHERE file_path = ?`, rel)
	return err
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
