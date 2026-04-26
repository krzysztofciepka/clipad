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
