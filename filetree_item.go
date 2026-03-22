package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type TreeNode struct {
	Name     string
	Path     string
	IsDir    bool
	Expanded bool
	Children []*TreeNode
}

type FlatItem struct {
	Node  *TreeNode
	Depth int
}

func buildTree(root string) (*TreeNode, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	node := &TreeNode{
		Name:  info.Name(),
		Path:  root,
		IsDir: true,
	}
	if err := populateChildren(node); err != nil {
		return nil, err
	}
	return node, nil
}

func populateChildren(node *TreeNode) error {
	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return err
	}

	var dirs, files []*TreeNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		childPath := filepath.Join(node.Path, name)
		if entry.IsDir() {
			child := &TreeNode{
				Name:  name,
				Path:  childPath,
				IsDir: true,
			}
			if err := populateChildren(child); err != nil {
				continue
			}
			if hasMarkdownFiles(child) {
				dirs = append(dirs, child)
			}
		} else if strings.HasSuffix(strings.ToLower(name), ".md") {
			files = append(files, &TreeNode{
				Name: name,
				Path: childPath,
			})
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	node.Children = append(dirs, files...)
	return nil
}

func hasMarkdownFiles(node *TreeNode) bool {
	for _, child := range node.Children {
		if !child.IsDir {
			return true
		}
		if hasMarkdownFiles(child) {
			return true
		}
	}
	return false
}

func flattenTree(node *TreeNode, depth int) []FlatItem {
	var items []FlatItem
	for _, child := range node.Children {
		items = append(items, FlatItem{Node: child, Depth: depth})
		if child.IsDir && child.Expanded {
			items = append(items, flattenTree(child, depth+1)...)
		}
	}
	return items
}

func collectFiles(node *TreeNode) []*TreeNode {
	var files []*TreeNode
	for _, child := range node.Children {
		if child.IsDir {
			files = append(files, collectFiles(child)...)
		} else {
			files = append(files, child)
		}
	}
	return files
}
