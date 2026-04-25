package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPickAsset_ExactMatch(t *testing.T) {
	rel := release{
		TagName: "v0.0.22",
		Assets: []releaseAsset{
			{Name: "clipad-v0.0.22-linux-amd64", BrowserDownloadURL: "https://example.test/a"},
		},
	}
	a, err := pickAsset(rel, "linux", "amd64")
	if err != nil {
		t.Fatalf("pickAsset: %v", err)
	}
	if a.Name != "clipad-v0.0.22-linux-amd64" {
		t.Fatalf("got %q", a.Name)
	}
}

func TestPickAsset_PicksMatchingFromMany(t *testing.T) {
	rel := release{
		TagName: "v0.0.22",
		Assets: []releaseAsset{
			{Name: "clipad-v0.0.22-darwin-arm64"},
			{Name: "clipad-v0.0.22-linux-amd64", BrowserDownloadURL: "https://example.test/want"},
			{Name: "checksums.txt"},
		},
	}
	a, err := pickAsset(rel, "linux", "amd64")
	if err != nil {
		t.Fatalf("pickAsset: %v", err)
	}
	if a.BrowserDownloadURL != "https://example.test/want" {
		t.Fatalf("picked wrong asset: %+v", a)
	}
}

func TestPickAsset_NoMatchReturnsError(t *testing.T) {
	rel := release{
		TagName: "v0.0.22",
		Assets:  []releaseAsset{{Name: "clipad-v0.0.22-darwin-arm64"}},
	}
	_, err := pickAsset(rel, "linux", "amd64")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "clipad-v0.0.22-linux-amd64") {
		t.Fatalf("error should mention expected asset name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "v0.0.22") {
		t.Fatalf("error should mention release tag, got: %v", err)
	}
}

func TestFetchLatestRelease_Success(t *testing.T) {
	body := `{
		"tag_name": "v0.0.22",
		"assets": [
			{"name": "clipad-v0.0.22-linux-amd64",
			 "browser_download_url": "https://example.test/clipad",
			 "size": 16355490,
			 "digest": "sha256:deadbeef"}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/krzysztofciepka/clipad/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept header = %q", got)
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "clipad-upgrader/") {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rel, err := fetchLatestRelease(ctx, srv.URL, "v0.0.20")
	if err != nil {
		t.Fatalf("fetchLatestRelease: %v", err)
	}
	if rel.TagName != "v0.0.22" {
		t.Fatalf("TagName = %q", rel.TagName)
	}
	if len(rel.Assets) != 1 || rel.Assets[0].Digest != "sha256:deadbeef" {
		t.Fatalf("assets = %+v", rel.Assets)
	}
}

func TestFetchLatestRelease_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"rate limit"}`)
	}))
	defer srv.Close()

	_, err := fetchLatestRelease(context.Background(), srv.URL, "v0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error should mention status, got: %v", err)
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error should include body snippet, got: %v", err)
	}
}

func TestFetchLatestRelease_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json {`)
	}))
	defer srv.Close()

	_, err := fetchLatestRelease(context.Background(), srv.URL, "v0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse release metadata") {
		t.Fatalf("error should mention parse failure, got: %v", err)
	}
}

func TestDownloadAsset_Success(t *testing.T) {
	payload := []byte("fake clipad binary contents")
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "clipad.new")
	if err := downloadAsset(context.Background(), srv.URL, dst, digest, "vTest"); err != nil {
		t.Fatalf("downloadAsset: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch")
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestDownloadAsset_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "clipad.new")
	err := downloadAsset(context.Background(), srv.URL, dst, "sha256:0", "vTest")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error should mention status, got: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("temp file should not exist, stat err = %v", err)
	}
}

func TestDownloadAsset_DigestMismatch(t *testing.T) {
	payload := []byte("payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	wrong := "sha256:" + strings.Repeat("0", 64)
	dst := filepath.Join(t.TempDir(), "clipad.new")
	err := downloadAsset(context.Background(), srv.URL, dst, wrong, "vTest")
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error should mention checksum mismatch, got: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("temp file should be cleaned up on mismatch, stat err = %v", err)
	}
}

func TestInstallBinary_HappyPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clipad")
	src := filepath.Join(dir, ".clipad-upgrade-1234")

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	if err := installBinary(src, target); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("target content = %q, want \"new\"", got)
	}

	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be moved away, stat err = %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestInstallBinary_PermissionErrorOnFirstRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clipad")
	src := filepath.Join(dir, ".clipad-upgrade-1234")

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	err := installBinary(src, target)
	if err == nil {
		t.Fatal("expected error on read-only dir")
	}
	if !strings.Contains(err.Error(), "move existing binary aside") {
		t.Fatalf("error should reference backup step, got: %v", err)
	}

	os.Chmod(dir, 0o755)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("target content changed: %q", got)
	}
}

func TestInstallBinary_RollbackOnSecondRenameFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clipad")
	src := filepath.Join(dir, ".clipad-upgrade-1234")

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	original := renameImpl
	calls := 0
	renameImpl = func(oldpath, newpath string) error {
		calls++
		if calls == 2 {
			return errors.New("simulated rename failure")
		}
		return original(oldpath, newpath)
	}
	t.Cleanup(func() { renameImpl = original })

	err := installBinary(src, target)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "install new binary") {
		t.Fatalf("error should mention install failure, got: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after rollback: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("rollback failed: target content = %q, want \"old\"", got)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old should not be left behind after successful rollback, stat err = %v", err)
	}
}
