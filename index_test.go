package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestChunkFile_SimpleParagraphs(t *testing.T) {
	in := "First paragraph line one.\nFirst paragraph line two.\n\nSecond paragraph.\n\nThird."
	got := chunkFile(in)
	want := []chunk{
		{StartLine: 1, EndLine: 2, Text: "First paragraph line one.\nFirst paragraph line two."},
		{StartLine: 4, EndLine: 4, Text: "Second paragraph."},
		{StartLine: 6, EndLine: 6, Text: "Third."},
	}
	for i := range want {
		want[i].Hash = chunkHash(want[i].Text)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chunkFile got %#v\nwant %#v", got, want)
	}
}

func TestChunkFile_Empty(t *testing.T) {
	if got := chunkFile(""); len(got) != 0 {
		t.Errorf("chunkFile(\"\") = %v, want empty", got)
	}
}

func TestChunkFile_WhitespaceOnly(t *testing.T) {
	if got := chunkFile("   \n\n\t\n"); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestChunkFile_OversizeParagraphSplits(t *testing.T) {
	line := strings.Repeat("a ", 300) // 600 chars
	para := line + "\n" + line + "\n" + line + "\n" + line + "\n" + line
	got := chunkFile(para)
	if len(got) < 2 {
		t.Fatalf("expected oversize paragraph to split into >=2 chunks, got %d", len(got))
	}
	for _, c := range got {
		if len(c.Text) > maxChunkChars+len(line) {
			t.Errorf("chunk too large: %d chars", len(c.Text))
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i].StartLine != got[i-1].EndLine+1 {
			t.Errorf("line gap: chunk[%d] starts at %d, prev ended at %d", i, got[i].StartLine, got[i-1].EndLine)
		}
	}
}

func TestOpenIndex_InMemory(t *testing.T) {
	idx, err := OpenIndex(":memory:", "/tmp/vault", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.db.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)`, "schema_version", "1"); err != nil {
		t.Fatal(err)
	}
	var v string
	if err := idx.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, "schema_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "1" {
		t.Errorf("got %q, want 1", v)
	}
}

func TestEncodeDecodeEmbedding_Roundtrip(t *testing.T) {
	in := []float32{0.1, -0.2, 3.14, 0}
	got := decodeEmbedding(encodeEmbedding(in))
	if !reflect.DeepEqual(got, in) {
		t.Errorf("got %v, want %v", got, in)
	}
}

func TestCosine(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	c := []float32{0, 1, 0}
	if cosine(a, b) != 1 {
		t.Errorf("parallel: got %v, want 1", cosine(a, b))
	}
	if cosine(a, c) != 0 {
		t.Errorf("orthogonal: got %v, want 0", cosine(a, c))
	}
}

// fakeEmbedder hands out deterministic vectors keyed by text.
type fakeEmbedder struct {
	model string
	dim   int
	calls int
	calc  func(text string) []float32
}

func (f *fakeEmbedder) Model() string { return f.model }
func (f *fakeEmbedder) Dim() int      { return f.dim }
func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.calc(t)
	}
	return out, nil
}

// onehotEmbedder returns a vector with 1 in slot=hash%dim and 0 elsewhere.
func onehotEmbedder(model string, dim int) *fakeEmbedder {
	return &fakeEmbedder{model: model, dim: dim, calc: func(text string) []float32 {
		v := make([]float32, dim)
		h := 0
		for _, b := range []byte(text) {
			h = (h*31 + int(b)) & 0x7fffffff
		}
		v[h%dim] = 1
		return v
	}}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRebuildFile_FreshFileEmbedsAllChunks(t *testing.T) {
	vault := t.TempDir()
	path := writeFile(t, vault, "a.md", "para one.\n\npara two.\n\npara three.")

	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	embedded, err := idx.RebuildFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if embedded != 3 {
		t.Errorf("RebuildFile embedded = %d, want 3", embedded)
	}

	var n int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("rows = %d, want 3", n)
	}
	if emb.calls != 1 {
		t.Errorf("embedder calls = %d, want 1 (one batch for the whole file)", emb.calls)
	}
}

func TestRebuildFile_OnlyChangedChunkReEmbeds(t *testing.T) {
	vault := t.TempDir()
	path := writeFile(t, vault, "a.md", "alpha.\n\nbeta.\n\ngamma.")

	emb := onehotEmbedder("m", 8)
	idx, _ := OpenIndex(":memory:", vault, emb)
	defer idx.Close()

	if _, err := idx.RebuildFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := emb.calls

	if err := os.WriteFile(path, []byte("alpha.\n\nbeta MODIFIED.\n\ngamma."), 0o644); err != nil {
		t.Fatal(err)
	}
	embedded, err := idx.RebuildFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if embedded != 1 {
		t.Errorf("incremental embed count = %d, want 1", embedded)
	}

	if emb.calls != callsAfterFirst+1 {
		t.Errorf("embedder calls after edit = %d, want %d", emb.calls, callsAfterFirst+1)
	}
	var n int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("rows after edit = %d, want 3", n)
	}
	var found int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE text LIKE ?`, "%MODIFIED%").Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Errorf("found = %d, want 1 row containing MODIFIED", found)
	}
}

func TestRemoveFile(t *testing.T) {
	vault := t.TempDir()
	pathA := writeFile(t, vault, "a.md", "x.\n\ny.")
	pathB := writeFile(t, vault, "b.md", "z.")
	emb := onehotEmbedder("m", 8)
	idx, _ := OpenIndex(":memory:", vault, emb)
	defer idx.Close()

	_, _ = idx.RebuildFile(context.Background(), pathA)
	_, _ = idx.RebuildFile(context.Background(), pathB)

	if err := idx.RemoveFile(context.Background(), pathA); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = ?`, "a.md").Scan(&n)
	if n != 0 {
		t.Errorf("a.md rows after remove = %d, want 0", n)
	}
	_ = idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = ?`, "b.md").Scan(&n)
	if n == 0 {
		t.Errorf("b.md rows after remove = 0, want >0")
	}
}

func TestSearch_RanksByCosine(t *testing.T) {
	vault := t.TempDir()
	a := writeFile(t, vault, "a.md", "alpha")
	b := writeFile(t, vault, "b.md", "beta")
	c := writeFile(t, vault, "c.md", "gamma")
	emb := onehotEmbedder("m", 16)
	idx, _ := OpenIndex(":memory:", vault, emb)
	defer idx.Close()

	for _, p := range []string{a, b, c} {
		if _, err := idx.RebuildFile(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}

	res, err := idx.Search(context.Background(), "alpha", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("len(res) = %d, want 2", len(res))
	}
	if res[0].Path != "a.md" {
		t.Errorf("top result = %q, want a.md", res[0].Path)
	}
	if res[0].Score < res[1].Score {
		t.Errorf("scores not descending: %v then %v", res[0].Score, res[1].Score)
	}
}

func TestPruneOrphans_RemovesChunksForMissingFiles(t *testing.T) {
	vault := t.TempDir()
	writeFile(t, vault, "alive.md", "para one.\n\npara two.")
	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	// Index both files, then delete one from disk.
	gone := writeFile(t, vault, "gone.md", "dead chunk.")
	if _, err := idx.RebuildFile(context.Background(), filepath.Join(vault, "alive.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.RebuildFile(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	removed, err := idx.PruneOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("pruned files = %d, want 1", removed)
	}

	var goneCount, aliveCount int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = 'gone.md'`).Scan(&goneCount); err != nil {
		t.Fatal(err)
	}
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = 'alive.md'`).Scan(&aliveCount); err != nil {
		t.Fatal(err)
	}
	if goneCount != 0 {
		t.Errorf("gone.md chunks = %d, want 0", goneCount)
	}
	if aliveCount == 0 {
		t.Errorf("alive.md chunks = 0, want > 0")
	}
}
