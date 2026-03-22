package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	var b strings.Builder

	end := tp.offset + tp.height
	if end > len(tp.items) {
		end = len(tp.items)
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

		if i == tp.cursor && focused {
			padded := line
			lineWidth := lipgloss.Width(padded)
			if lineWidth < tp.width-2 {
				padded += strings.Repeat(" ", tp.width-2-lineWidth)
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

	return treePanelStyle.Width(tp.width).Height(tp.height).Render(rendered)
}
