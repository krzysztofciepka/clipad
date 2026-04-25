package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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

func downloadAsset(ctx context.Context, url, dstPath, expectedDigest, version string) (retErr error) {
	defer func() {
		if retErr != nil {
			os.Remove(dstPath)
		}
	}()

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

func runUpgrade(out io.Writer, currentVersion, apiBaseURL, exePath string) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("self-upgrade is not supported on %s/%s — please reinstall manually", runtime.GOOS, runtime.GOARCH)
	}

	target, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		// EvalSymlinks fails for non-existent paths; fall back to the raw exe
		// path so synthetic test paths still work.
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
		if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
			return fmt.Errorf("cannot write to %s: %w — re-run with sudo or move clipad to a user-owned path", dir, err)
		}
		return err
	}
	fmt.Fprintln(out, "Verifying checksum... ok")

	fmt.Fprintf(out, "Installing to %s... ", target)
	if err := installBinary(tmpPath, target); err != nil {
		fmt.Fprintln(out, "failed")
		os.Remove(tmpPath)
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
