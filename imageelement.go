package main

import (
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
