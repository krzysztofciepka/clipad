package main

import (
	"github.com/sahilm/fuzzy"
)

type fileSource []*TreeNode

func (f fileSource) String(i int) string {
	return f[i].Name
}

func (f fileSource) Len() int {
	return len(f)
}

func filterFiles(files []*TreeNode, query string) []*TreeNode {
	if query == "" {
		return files
	}
	src := fileSource(files)
	matches := fuzzy.FindFrom(query, src)
	results := make([]*TreeNode, len(matches))
	for i, m := range matches {
		results[i] = files[m.Index]
	}
	return results
}
