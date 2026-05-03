package main

import (
	"io/fs"
	"path/filepath"
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
