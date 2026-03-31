package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	treePanelStyle = lipgloss.NewStyle().
		Padding(0, 1).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder())

	treeSelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	treeDirStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("75")).
		Bold(true)

	treeFileStyle  = lipgloss.NewStyle()
	treeActiveFile = lipgloss.NewStyle().
			Foreground(lipgloss.Color("156"))
)

type TreePanel struct {
	root        *TreeNode
	items       []FlatItem
	cursor      int
	offset      int
	height      int
	width       int
	currentFile string
}

func newTreePanel(root *TreeNode, width, height int) TreePanel {
	tp := TreePanel{
		root:   root,
		width:  width,
		height: height,
	}
	tp.rebuildItems()
	return tp
}

func (tp *TreePanel) rebuildItems() {
	if tp.root != nil {
		tp.items = flattenTree(tp.root, 0)
	} else {
		tp.items = nil
	}
}

func (tp *TreePanel) clampOffset() {
	// Clamp cursor to valid range
	if tp.cursor >= len(tp.items) {
		tp.cursor = len(tp.items) - 1
	}
	if tp.cursor < 0 {
		tp.cursor = 0
	}

	// Prevent offset from leaving blank space at the bottom
	if tp.height > 0 && len(tp.items) > 0 {
		maxOffset := len(tp.items) - tp.height
		if maxOffset < 0 {
			maxOffset = 0
		}
		if tp.offset > maxOffset {
			tp.offset = maxOffset
		}
	}

	// Ensure cursor is visible
	if tp.cursor < tp.offset {
		tp.offset = tp.cursor
	}
	if tp.height > 0 && tp.cursor >= tp.offset+tp.height {
		tp.offset = tp.cursor - tp.height + 1
	}
	if tp.offset < 0 {
		tp.offset = 0
	}
}

func (tp *TreePanel) moveUp() {
	if tp.cursor > 0 {
		tp.cursor--
		if tp.cursor < tp.offset {
			tp.offset = tp.cursor
		}
	}
}

func (tp *TreePanel) moveDown() {
	if tp.cursor < len(tp.items)-1 {
		tp.cursor++
		if tp.cursor >= tp.offset+tp.height {
			tp.offset = tp.cursor - tp.height + 1
		}
	}
}

func (tp *TreePanel) toggleOrSelect() *TreeNode {
	if tp.cursor >= len(tp.items) {
		return nil
	}
	item := tp.items[tp.cursor]
	if item.Node.IsDir {
		item.Node.Expanded = !item.Node.Expanded
		tp.rebuildItems()
		if tp.cursor >= len(tp.items) {
			tp.cursor = len(tp.items) - 1
		}
		return nil
	}
	return item.Node
}

func (tp *TreePanel) selectedNode() *TreeNode {
	if tp.cursor >= 0 && tp.cursor < len(tp.items) {
		return tp.items[tp.cursor].Node
	}
	return nil
}

func (tp TreePanel) View(focused bool) string {
	if tp.width <= 0 || tp.height <= 0 {
		return ""
	}

	var b strings.Builder

	end := tp.offset + tp.height
	if end > len(tp.items) {
		end = len(tp.items)
	}

	// Content area width (treePanelStyle has Padding(0,1) = 2 chars horizontal)
	maxW := tp.width - 2
	if maxW < 1 {
		maxW = 1
	}

	for i := tp.offset; i < end; i++ {
		item := tp.items[i]
		indent := strings.Repeat("  ", item.Depth)

		var icon, name string
		if item.Node.IsDir {
			if item.Node.Expanded {
				icon = "▼ "
			} else {
				icon = "▶ "
			}
			name = treeDirStyle.Render(item.Node.Name)
		} else {
			icon = "  "
			if item.Node.Path == tp.currentFile {
				name = treeActiveFile.Render(item.Node.Name)
			} else {
				name = treeFileStyle.Render(item.Node.Name)
			}
		}

		line := fmt.Sprintf("%s%s%s", indent, icon, name)

		if lipgloss.Width(line) > maxW {
			line = ansi.Truncate(line, maxW, "…")
		}

		if i == tp.cursor && focused {
			padded := line
			lineWidth := lipgloss.Width(padded)
			if lineWidth < maxW {
				padded += strings.Repeat(" ", maxW-lineWidth)
			}
			line = treeSelectedStyle.Render(padded)
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	rendered := b.String()
	lineCount := end - tp.offset
	for i := lineCount; i < tp.height; i++ {
		rendered += "\n"
	}

	return treePanelStyle.Width(tp.width).MaxHeight(tp.height).Render(rendered)
}
