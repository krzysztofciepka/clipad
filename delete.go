package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// countTreeContents walks root and returns the number of files and the number
// of subdirectories beneath it. The root directory itself is not counted. Both
// markdown and non-markdown files are counted because the count reflects what
// os.RemoveAll will actually delete from the filesystem, not what the tree
// view renders.
func countTreeContents(root string) (files, folders int, err error) {
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			folders++
		} else {
			files++
		}
		return nil
	})
	return files, folders, walkErr
}

// pathIsInside reports whether path is equal to root or located somewhere
// beneath it. Both arguments must already be cleaned absolute paths in the
// same form os.ReadDir / filepath.Join produce, which is the case throughout
// the model (vault, node.Path, currentFile are all built from the vault root
// by filepath.Join).
func pathIsInside(path, root string) bool {
	if path == root {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(path, prefix)
}
