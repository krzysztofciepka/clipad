# Self-upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `clipad --upgrade` (download latest release, sha256-verify, atomically swap binary in place) and `clipad --version` (print embedded tag) flags.

**Architecture:** All upgrade logic lives in a new `upgrade.go` file as small, independently testable functions (`pickAsset`, `fetchLatestRelease`, `downloadAsset`, `installBinary`, `runUpgrade`). `main.go` gets a flag-parsing prologue that dispatches `--version` / `--upgrade` before the TUI starts. The compile-time `version` variable is overridden via `-ldflags "-X main.version=<tag>"` in the release build.

**Tech Stack:** Go (stdlib only — `net/http`, `crypto/sha256`, `flag`, `context`, etc.). Test style matches the rest of the project: `package main` table-driven tests in `*_test.go` files using `httptest.Server` for HTTP stubs and `t.TempDir()` for filesystem isolation.

**Spec:** `docs/superpowers/specs/2026-04-25-self-upgrade-design.md`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `upgrade.go` (new) | All upgrade-feature code: `release`/`releaseAsset` types, `pickAsset`, `fetchLatestRelease`, `downloadAsset`, `installBinary`, `runUpgrade`. Stdlib-only, no TUI imports. The package-level `var renameImpl = os.Rename` is exposed only for testing the rollback path. |
| `upgrade_test.go` (new) | Hermetic unit tests using `httptest.Server` for the GitHub API and asset host, plus `t.TempDir()` for the install-path tests. |
| `main.go` (modify) | Add `var version = "dev"`. Add a `flag` parsing prologue at the top of `main()` that handles `--version` (print and exit) and `--upgrade` (call `runUpgrade(os.Stderr, version, "https://api.github.com")` and exit). Existing TUI code runs only when neither flag was given. |
| `README.md` (modify) | Document `--version` and `--upgrade` flags. Update the build instructions to show the `-ldflags` invocation that embeds the release tag. |

---

## Task 1: Release types + `pickAsset`

**Files:**
- Create: `upgrade.go`
- Create: `upgrade_test.go`

- [ ] **Step 1: Write failing tests for `pickAsset`**

Create `upgrade_test.go`:

```go
package main

import (
	"strings"
	"testing"
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestPickAsset -v`
Expected: FAIL with "undefined: release" / "undefined: pickAsset".

- [ ] **Step 3: Implement types and `pickAsset`**

Create `upgrade.go`:

```go
package main

import (
	"fmt"
)

const (
	repoOwner = "krzysztofciepka"
	repoName  = "clipad"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func pickAsset(rel release, goos, goarch string) (releaseAsset, error) {
	want := fmt.Sprintf("clipad-%s-%s-%s", rel.TagName, goos, goarch)
	for _, a := range rel.Assets {
		if a.Name == want {
			return a, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("no asset matching %s in release %s", want, rel.TagName)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestPickAsset -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add upgrade.go upgrade_test.go
git commit -m "feat(upgrade): add release types and pickAsset"
```

---

## Task 2: `fetchLatestRelease`

**Files:**
- Modify: `upgrade.go`
- Modify: `upgrade_test.go`

- [ ] **Step 1: Write failing tests for `fetchLatestRelease`**

Append to `upgrade_test.go`:

```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"
)

// (merge this import with the existing import block — Go won't allow two)

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
```

(Merge the new imports `context`, `net/http`, `net/http/httptest`, `time` into the existing import block at the top of `upgrade_test.go`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestFetchLatestRelease -v`
Expected: FAIL with "undefined: fetchLatestRelease".

- [ ] **Step 3: Implement `fetchLatestRelease`**

Append to `upgrade.go`:

```go
import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// (merge this import with the existing import block at the top of upgrade.go)

const userAgentPrefix = "clipad-upgrader/"

func fetchLatestRelease(ctx context.Context, apiBaseURL, version string) (release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBaseURL, repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgentPrefix+version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return release{}, fmt.Errorf("read release metadata: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("failed to fetch latest release: %d: %s", resp.StatusCode, snippet(body))
	}

	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return release{}, fmt.Errorf("failed to parse release metadata: %w", err)
	}
	return rel, nil
}

func snippet(b []byte) string {
	const max = 200
	s := string(b)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestFetchLatestRelease -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add upgrade.go upgrade_test.go
git commit -m "feat(upgrade): add fetchLatestRelease with API stub tests"
```

---

## Task 3: `downloadAsset` (stream + sha256 verify)

**Files:**
- Modify: `upgrade.go`
- Modify: `upgrade_test.go`

- [ ] **Step 1: Write failing tests for `downloadAsset`**

Append to `upgrade_test.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// (merge into the existing import block)

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestDownloadAsset -v`
Expected: FAIL with "undefined: downloadAsset".

- [ ] **Step 3: Implement `downloadAsset`**

Append to `upgrade.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

// (merge into the existing import block)

func downloadAsset(ctx context.Context, url, dstPath, expectedDigest, version string) (retErr error) {
	cleanup := func() {
		if retErr != nil {
			os.Remove(dstPath)
		}
	}
	defer cleanup()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgentPrefix+version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: %d", url, resp.StatusCode)
	}

	f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", dstPath, err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("download interrupted: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dstPath, err)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	want := strings.TrimPrefix(expectedDigest, "sha256:")
	if got != want {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", want, got)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestDownloadAsset -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add upgrade.go upgrade_test.go
git commit -m "feat(upgrade): add downloadAsset with sha256 verification"
```

---

## Task 4: `installBinary` (atomic swap + rollback)

**Files:**
- Modify: `upgrade.go`
- Modify: `upgrade_test.go`

- [ ] **Step 1: Write failing tests for `installBinary`**

Append to `upgrade_test.go`:

```go
import "errors"

// (merge into the existing import block)

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

	// Original target should still be there with original content.
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

	// Override renameImpl to fail the second call only.
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestInstallBinary -v`
Expected: FAIL with "undefined: installBinary" / "undefined: renameImpl".

- [ ] **Step 3: Implement `installBinary` and `renameImpl`**

Append to `upgrade.go`:

```go
// renameImpl is os.Rename, exposed as a package-level variable so tests can
// inject a failure into the rollback path. Production code never reassigns it.
var renameImpl = os.Rename

func installBinary(srcPath, targetPath string) error {
	backup := targetPath + ".old"
	if err := renameImpl(targetPath, backup); err != nil {
		return fmt.Errorf("cannot move existing binary aside: %w", err)
	}
	if err := renameImpl(srcPath, targetPath); err != nil {
		if rerr := renameImpl(backup, targetPath); rerr != nil {
			return fmt.Errorf("failed to install new binary: %w; original saved at %s — restore manually", err, backup)
		}
		return fmt.Errorf("failed to install new binary: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestInstallBinary -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add upgrade.go upgrade_test.go
git commit -m "feat(upgrade): add installBinary with atomic swap and rollback"
```

---

## Task 5: `runUpgrade` orchestrator

**Files:**
- Modify: `upgrade.go`
- Modify: `upgrade_test.go`

- [ ] **Step 1: Write failing tests for `runUpgrade`**

Append to `upgrade_test.go`:

```go
import (
	"bytes"
	"runtime"
)

// (merge into the existing import block)

// fakeRelease writes API + asset stubs to httptest servers and returns the API URL.
func fakeRelease(t *testing.T, tag string, payload []byte) string {
	t.Helper()
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	t.Cleanup(assetSrv.Close)

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":"clipad-%s-%s-%s","browser_download_url":%q,"size":%d,"digest":%q}]}`,
			tag, tag, runtime.GOOS, runtime.GOARCH, assetSrv.URL+"/asset", len(payload), digest)
	}))
	t.Cleanup(apiSrv.Close)
	return apiSrv.URL
}

func TestRunUpgrade_AlreadyLatest(t *testing.T) {
	requests := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"tag_name":"v0.0.42","assets":[]}`)
	}))
	defer apiSrv.Close()

	exePath, _ := os.Executable()
	var out bytes.Buffer
	if err := runUpgrade(&out, "v0.0.42", apiSrv.URL, exePath); err != nil {
		t.Fatalf("runUpgrade: %v", err)
	}
	if requests != 1 {
		t.Fatalf("API requests = %d, want 1", requests)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("output should mention up-to-date, got: %q", out.String())
	}
}

func TestRunUpgrade_DevBuildAlwaysProceeds(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clipad")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	apiURL := fakeRelease(t, "v0.0.99", []byte("new binary bytes"))

	var out bytes.Buffer
	if err := runUpgrade(&out, "dev", apiURL, target); err != nil {
		t.Fatalf("runUpgrade: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "new binary bytes" {
		t.Fatalf("target not replaced: %q", got)
	}
}

func TestRunUpgrade_FullPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clipad")
	if err := os.WriteFile(target, []byte("v0.0.20-bytes"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	payload := []byte("v0.0.21-bytes")
	apiURL := fakeRelease(t, "v0.0.21", payload)

	var out bytes.Buffer
	if err := runUpgrade(&out, "v0.0.20", apiURL, target); err != nil {
		t.Fatalf("runUpgrade: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("target = %q, want %q", got, payload)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old should be removed")
	}
	if !strings.Contains(out.String(), "v0.0.20") || !strings.Contains(out.String(), "v0.0.21") {
		t.Fatalf("output should mention both versions, got: %q", out.String())
	}
}

func TestRunUpgrade_UnsupportedPlatform(t *testing.T) {
	// Force an unsupported platform by passing a target path on a non-existent
	// system; the platform gate runs first based on runtime.GOOS/GOARCH, so we
	// only need to assert the gate exists when GOOS/GOARCH != linux/amd64.
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		t.Skip("running on linux/amd64; unsupported-platform path not reachable here")
	}
	var out bytes.Buffer
	err := runUpgrade(&out, "v0.0.20", "http://unused", "/unused")
	if err == nil {
		t.Fatal("expected unsupported-platform error")
	}
	if !strings.Contains(err.Error(), "self-upgrade is not supported") {
		t.Fatalf("error should mention unsupported, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestRunUpgrade -v`
Expected: FAIL with "undefined: runUpgrade".

- [ ] **Step 3: Implement `runUpgrade`**

Append to `upgrade.go`:

```go
import (
	"path/filepath"
	"runtime"
	"time"
)

// (merge into the existing import block)

func runUpgrade(out io.Writer, currentVersion, apiBaseURL, exePath string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("self-upgrade is not supported on %s/%s — please reinstall manually", runtime.GOOS, runtime.GOARCH)
	}

	target, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		// EvalSymlinks fails for non-existent paths; fall back to the raw exe
		// path so unit tests with synthetic paths still work.
		target = exePath
	}
	dir := filepath.Dir(target)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rel, err := fetchLatestRelease(ctx, apiBaseURL, currentVersion)
	if err != nil {
		return err
	}

	if currentVersion != "dev" && rel.TagName == currentVersion {
		fmt.Fprintf(out, "clipad is up to date (%s).\n", currentVersion)
		return nil
	}

	asset, err := pickAsset(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Current version: %s\n", currentVersion)
	fmt.Fprintf(out, "Latest version:  %s\n", rel.TagName)
	fmt.Fprintf(out, "Downloading %s (%s)...\n", asset.Name, humanSize(asset.Size))

	tmpPath := filepath.Join(dir, fmt.Sprintf(".clipad-upgrade-%d", os.Getpid()))
	if err := downloadAsset(ctx, asset.BrowserDownloadURL, tmpPath, asset.Digest, currentVersion); err != nil {
		// Distinguish "can't write to dir" from other download errors so the
		// permissions hint reaches the user.
		if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
			return fmt.Errorf("cannot write to %s: %w — re-run with sudo or move clipad to a user-owned path", dir, err)
		}
		return err
	}
	fmt.Fprintln(out, "Verifying checksum... ok")

	fmt.Fprintf(out, "Installing to %s... ", target)
	if err := installBinary(tmpPath, target); err != nil {
		fmt.Fprintln(out, "failed")
		os.Remove(tmpPath) // installBinary may have already succeeded at the first rename; ensure no orphan
		return err
	}
	fmt.Fprintln(out, "ok")

	fmt.Fprintf(out, "Upgraded %s → %s. Restart clipad to use the new version.\n", currentVersion, rel.TagName)
	return nil
}

func humanSize(n int64) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestRunUpgrade -v`
Expected: PASS (3 tests run on linux/amd64; `TestRunUpgrade_UnsupportedPlatform` skips).

- [ ] **Step 5: Run full test suite to confirm no regressions**

Run: `go test ./... -v`
Expected: All tests pass (existing + new upgrade tests).

- [ ] **Step 6: Commit**

```bash
git add upgrade.go upgrade_test.go
git commit -m "feat(upgrade): add runUpgrade orchestrator"
```

---

## Task 6: Wire `--version` and `--upgrade` into `main.go`

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add `version` variable and flag dispatch**

Edit `main.go`:

Add the import `flag` to the existing import block (alphabetical position — between `fmt` and `os`).

Add immediately after the import block (before `type setupModel struct`):

```go
// version is overridden at release build time via:
//   go build -ldflags "-X main.version=vX.Y.Z" .
var version = "dev"
```

Replace the body of `func main()` at the top with the flag-dispatch prologue. The new `main()` reads:

```go
func main() {
	var (
		showVersion bool
		doUpgrade   bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&doUpgrade, "upgrade", false, "fetch the latest release and replace this binary")
	flag.Parse()

	if showVersion {
		fmt.Printf("clipad %s\n", version)
		return
	}
	if doUpgrade {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot determine clipad binary path: %v\n", err)
			os.Exit(1)
		}
		if err := runUpgrade(os.Stderr, version, "https://api.github.com", exe); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		setup := newSetupModel()
		p := tea.NewProgram(setup, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		sm := result.(setupModel)
		if !sm.done {
			os.Exit(0)
		}
		cfg = Config{Vault: sm.vault}
	}

	if _, err := os.Stat(cfg.Vault); err != nil {
		fmt.Fprintf(os.Stderr, "Vault directory not found: %s\n", cfg.Vault)
		os.Exit(1)
	}

	plugins := []Plugin{
		&BlackboxPlugin{},
		&OpenRouterPlugin{},
	}
	m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider)

	if m.treeRoot != nil && len(collectFiles(m.treeRoot)) == 0 {
		os.WriteFile(filepath.Join(cfg.Vault, "welcome.md"),
			[]byte("# Welcome to Clipad\n\nStart writing your notes here.\n"), 0o644)
		m.refreshTree()
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify build and `--version`**

Run: `go build -o /tmp/clipad-test .`
Expected: build succeeds.

Run: `/tmp/clipad-test --version`
Expected: prints `clipad dev`.

Run: `go build -ldflags "-X main.version=v0.0.42" -o /tmp/clipad-test .`
Run: `/tmp/clipad-test --version`
Expected: prints `clipad v0.0.42`.

- [ ] **Step 3: Verify `--upgrade` against the live API**

Run: `go build -ldflags "-X main.version=v0.0.20" -o /tmp/clipad-test .`
Run: `/tmp/clipad-test --upgrade`
Expected: stderr shows `Current version: v0.0.20`, `Latest version: v0.0.21` (or whatever is current), download progress, "ok" lines, and a final "Upgraded ..." line. The temp binary at `/tmp/clipad-test` is replaced with the new release.

Run: `/tmp/clipad-test --version`
Expected: prints whatever the new tag is, confirming the swap.

Run: `/tmp/clipad-test --upgrade`
Expected: prints `clipad is up to date (vX.Y.Z).` and exits 0 without downloading.

Clean up: `rm /tmp/clipad-test`

- [ ] **Step 4: Run the full test suite**

Run: `go test ./... -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat: add --version and --upgrade flags"
```

---

## Task 7: Document `--upgrade`, `--version`, and the release build command

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the Install section**

In `README.md`, replace the existing "From release" subsection with:

```markdown
### From release

Download the binary for your platform from the [latest release](https://github.com/krzysztofciepka/clipad/releases) and place it in your `PATH`.

To upgrade an existing installation in place:

```bash
clipad --upgrade
```

This downloads the latest release, verifies its sha256 checksum, and atomically replaces the running binary.
```

- [ ] **Step 2: Update the Usage section**

Add a new subsection after the existing "Usage" content:

```markdown
### CLI flags

| Flag | Action |
|------|--------|
| `--version` | Print the embedded version and exit |
| `--upgrade` | Fetch the latest GitHub release, verify its sha256, and replace the current binary in place. Restart clipad afterwards. Linux/amd64 only. |
```

- [ ] **Step 3: Update the build instructions**

In `README.md`, replace the manual-build instructions with:

````markdown
Or build manually:

```bash
git clone https://github.com/krzysztofciepka/clipad.git
cd clipad
go build -o clipad .
```

For a release build that knows its own version (so `--version` and `--upgrade` work correctly):

```bash
TAG=v0.0.22
go build -ldflags "-X main.version=$TAG" -o clipad-$TAG-linux-amd64 .
```
````

- [ ] **Step 4: Verify the README renders cleanly**

Run: `grep -n "upgrade\|--version\|ldflags" README.md`
Expected: each new line is present once and the surrounding text reads naturally.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document --upgrade, --version, and release build command"
```

---

## Final Verification

- [ ] Run `go test ./... -v` — all tests pass.
- [ ] Run `go vet ./...` — no warnings.
- [ ] Manually verify `--version` and `--upgrade` against the live repo as in Task 6, Step 3.
- [ ] Confirm `git status` is clean and `git log --oneline -10` shows the seven commits in order.
