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
	"strings"
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
