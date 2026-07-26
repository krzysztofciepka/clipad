package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

// imageElementRe matches a whole line that is exactly a markdown image:
// optional surrounding whitespace, ![any alt](target), nothing else.
var imageElementRe = regexp.MustCompile(`^\s*!\[[^\]]*\]\(([^)]+)\)\s*$`)

// parseImageElement reports whether line is a lone markdown image whose target
// has a recognized image extension, returning the target path.
func parseImageElement(line string) (string, bool) {
	m := imageElementRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	target := m[1]
	if !imageExtensions[strings.ToLower(filepath.Ext(target))] {
		return "", false
	}
	return target, true
}

func assetFilename(data []byte, date string) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("img-%s-%s.png", date, hex.EncodeToString(sum[:])[:8])
}

func saveAsset(vault string, data []byte, date string) (string, error) {
	dir := filepath.Join(vault, "assets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs := filepath.Join(dir, assetFilename(data, date))
	if _, err := os.Stat(abs); err == nil {
		return abs, nil // already stored (content-addressed)
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", err
	}
	return abs, nil
}

func assetRelPath(noteDir, assetAbs string) string {
	rel, err := filepath.Rel(noteDir, assetAbs)
	if err != nil {
		return assetAbs
	}
	return filepath.ToSlash(rel)
}

func resolveAssetPath(noteDir, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(noteDir, filepath.FromSlash(target))
}
