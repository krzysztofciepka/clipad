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

	tea "github.com/charmbracelet/bubbletea"
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
	// Serialize all access through a single connection. The agent goroutine
	// reads/writes the index (search prune + queries) concurrently with the
	// background indexing sweep; a single connection avoids "database is locked"
	// errors and also keeps a ":memory:" DB consistent across queries.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("busy_timeout: %w", err)
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
// chunks whose hash isn't already present (model-matched). Returns the
// number of chunks that were freshly embedded.
func (idx *Index) RebuildFile(ctx context.Context, absPath string) (int, error) {
	if idx.embedder == nil {
		return 0, errors.New("index: no embedder configured")
	}
	rel, err := idx.relPath(absPath)
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	newChunks := chunkFile(string(data))

	model := idx.embedder.Model()
	existing := map[string]int64{}
	rows, err := idx.db.QueryContext(ctx,
		`SELECT id, chunk_hash FROM chunks WHERE file_path = ? AND model = ?`, rel, model)
	if err != nil {
		return 0, fmt.Errorf("select existing: %w", err)
	}
	for rows.Next() {
		var id int64
		var h string
		if err := rows.Scan(&id, &h); err != nil {
			rows.Close()
			return 0, err
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
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	delRows, err := tx.QueryContext(ctx,
		`SELECT id FROM chunks WHERE file_path = ? AND model = ?`, rel, model)
	if err != nil {
		return 0, fmt.Errorf("select for delete: %w", err)
	}
	var idsToDelete []int64
	for delRows.Next() {
		var id int64
		if err := delRows.Scan(&id); err != nil {
			delRows.Close()
			return 0, err
		}
		if !keep[id] {
			idsToDelete = append(idsToDelete, id)
		}
	}
	delRows.Close()
	for _, id := range idsToDelete {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE id = ?`, id); err != nil {
			return 0, fmt.Errorf("delete: %w", err)
		}
	}

	embedded := 0
	if len(toEmbed) > 0 {
		texts := make([]string, len(toEmbed))
		for i, c := range toEmbed {
			texts[i] = c.Text
		}
		vecs, err := idx.embedder.Embed(ctx, texts)
		if err != nil {
			return 0, fmt.Errorf("embed: %w", err)
		}
		if len(vecs) != len(toEmbed) {
			return 0, fmt.Errorf("embed: got %d vectors for %d chunks", len(vecs), len(toEmbed))
		}
		now := time.Now().Unix()
		for i, c := range toEmbed {
			blob := encodeEmbedding(vecs[i])
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO chunks(file_path, start_line, end_line, text, chunk_hash, embedding, model, dim, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				rel, c.StartLine, c.EndLine, c.Text, c.Hash, blob, model, len(vecs[i]), now); err != nil {
				return 0, fmt.Errorf("insert: %w", err)
			}
		}
		embedded = len(toEmbed)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return embedded, nil
}

// Result is a single search hit with its score.
type Result struct {
	Path      string  // relative to vault
	StartLine int
	EndLine   int
	Text      string
	Score     float32
}

// IsSearchable reports whether the index has an embedder configured and can
// serve semantic searches.
func (idx *Index) IsSearchable() bool {
	return idx.embedder != nil
}

// Search embeds the query and returns the top-k chunks by cosine similarity,
// restricted to rows that match the embedder's current model.
func (idx *Index) Search(ctx context.Context, query string, k int) ([]Result, error) {
	if idx.embedder == nil {
		return nil, errors.New("index: no embedder configured")
	}
	if k <= 0 {
		return nil, nil
	}
	vecs, err := idx.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embed query: got %d vectors", len(vecs))
	}
	q := vecs[0]
	model := idx.embedder.Model()

	rows, err := idx.db.QueryContext(ctx,
		`SELECT file_path, start_line, end_line, text, embedding FROM chunks WHERE model = ?`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		r     Result
		score float32
	}
	var all []scored
	for rows.Next() {
		var r Result
		var blob []byte
		if err := rows.Scan(&r.Path, &r.StartLine, &r.EndLine, &r.Text, &blob); err != nil {
			return nil, err
		}
		v := decodeEmbedding(blob)
		s := cosine(q, v)
		r.Score = s
		all = append(all, scored{r: r, score: s})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by descending score (simple selection — vault-scale).
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if all[j].score > all[i].score {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	if k > len(all) {
		k = len(all)
	}
	out := make([]Result, k)
	for i := 0; i < k; i++ {
		out[i] = all[i].r
	}
	return out, nil
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

// PruneOrphans deletes all chunks whose file_path no longer exists under the
// vault on disk. Returns the number of distinct files pruned. Cheap: one stat
// per distinct file, no embedding calls.
func (idx *Index) PruneOrphans(ctx context.Context) (int, error) {
	rows, err := idx.db.QueryContext(ctx, `SELECT DISTINCT file_path FROM chunks`)
	if err != nil {
		return 0, fmt.Errorf("select distinct paths: %w", err)
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan file_path: %w", err)
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate file_paths: %w", err)
	}
	rows.Close()

	// Only a definitive "does not exist" prunes a file. Any other stat error
	// (permissions, I/O) is treated conservatively as "still present" so we
	// never delete chunks for a file that may exist.
	removed := 0
	for _, rel := range paths {
		if _, err := os.Stat(filepath.Join(idx.vault, rel)); os.IsNotExist(err) {
			if _, err := idx.db.ExecContext(ctx, `DELETE FROM chunks WHERE file_path = ?`, rel); err != nil {
				return removed, fmt.Errorf("delete %q: %w", rel, err)
			}
			removed++
		}
	}
	return removed, nil
}

type indexProgressMsg struct {
	done, total int
	embedded    int  // cumulative chunks freshly embedded this sweep
	idx         *Index
	paths       []string
}
type indexDoneMsg struct {
	err      error
	embedded int
}
type indexFileMsg struct{ path string }

// collectMarkdownFiles walks vault and returns absolute paths of every .md.
func collectMarkdownFiles(vault string) ([]string, error) {
	var out []string
	err := filepath.Walk(vault, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && path != vault) || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// startInitialIndex returns a tea.Cmd that walks the vault and queues the
// first per-file processing step. Subsequent files chain via indexProgressMsg.
func startInitialIndex(idx *Index, vault string) tea.Cmd {
	return func() tea.Msg {
		if idx == nil || idx.embedder == nil {
			return indexDoneMsg{}
		}
		paths, err := collectMarkdownFiles(vault)
		if err != nil {
			return indexDoneMsg{err: err}
		}
		if len(paths) == 0 {
			return indexDoneMsg{}
		}
		// Emit a "starting" progress msg; the model handler chains the next file.
		return indexProgressMsg{done: 0, total: len(paths), embedded: 0, idx: idx, paths: paths}
	}
}

// processIndexFileCmd runs RebuildFile on paths[i] and emits the next progress msg.
// When i == len(paths), it emits indexDoneMsg.
func processIndexFileCmd(idx *Index, paths []string, i, embeddedSoFar int) tea.Cmd {
	return func() tea.Msg {
		if i >= len(paths) {
			return indexDoneMsg{embedded: embeddedSoFar}
		}
		n, err := idx.RebuildFile(context.Background(), paths[i])
		if err != nil {
			return indexDoneMsg{err: err, embedded: embeddedSoFar}
		}
		return indexProgressMsg{
			done:     i + 1,
			total:    len(paths),
			embedded: embeddedSoFar + n,
			idx:      idx,
			paths:    paths,
		}
	}
}

// reindexFileCmd re-indexes a single file in the background.
func reindexFileCmd(idx *Index, path string) tea.Cmd {
	return func() tea.Msg {
		if idx == nil || idx.embedder == nil {
			return indexFileMsg{path: path}
		}
		_, _ = idx.RebuildFile(context.Background(), path)
		return indexFileMsg{path: path}
	}
}

// removeFileFromIndexCmd handles deletion.
func removeFileFromIndexCmd(idx *Index, path string) tea.Cmd {
	return func() tea.Msg {
		if idx == nil {
			return indexFileMsg{path: path}
		}
		_ = idx.RemoveFile(context.Background(), path)
		return indexFileMsg{path: path}
	}
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
