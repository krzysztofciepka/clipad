package main

import (
	"strings"
	"testing"
)

func makeTreePanel(numItems, height int) TreePanel {
	tp := TreePanel{height: height, width: 20}
	tp.items = make([]FlatItem, numItems)
	for i := range tp.items {
		tp.items[i] = FlatItem{Node: &TreeNode{Name: "f", IsDir: false}, Depth: 0}
	}
	return tp
}

func TestItemsHeight_AccountsForPinnedRow(t *testing.T) {
	tp := makeTreePanel(0, 10)
	if got := tp.itemsHeight(); got != 9 {
		t.Errorf("itemsHeight() = %d, want 9", got)
	}
	tp.height = 1
	if got := tp.itemsHeight(); got != 0 {
		t.Errorf("itemsHeight() height=1 = %d, want 0", got)
	}
	tp.height = 0
	if got := tp.itemsHeight(); got != 0 {
		t.Errorf("itemsHeight() height=0 = %d, want 0", got)
	}
}

func TestScrollBy_DecouplesFromCursor(t *testing.T) {
	tp := makeTreePanel(50, 10) // itemsHeight = 9
	tp.cursor = 0
	tp.offset = 0
	tp.scrollBy(20)
	if tp.offset != 20 {
		t.Errorf("offset after scrollBy(20) = %d, want 20", tp.offset)
	}
	if tp.cursor != 0 {
		t.Errorf("cursor moved during scrollBy: cursor = %d, want 0", tp.cursor)
	}
}

func TestScrollBy_ClampsAtBounds(t *testing.T) {
	tp := makeTreePanel(50, 10) // itemsHeight = 9, max offset = 50 - 9 = 41
	tp.scrollBy(1000)
	if tp.offset != 41 {
		t.Errorf("scrollBy(1000) offset = %d, want 41", tp.offset)
	}
	tp.scrollBy(-1000)
	if tp.offset != 0 {
		t.Errorf("scrollBy(-1000) offset = %d, want 0", tp.offset)
	}
}

func TestScrollBy_FewerItemsThanHeight(t *testing.T) {
	tp := makeTreePanel(3, 10) // itemsHeight = 9, items < height
	tp.offset = 0
	tp.scrollBy(5)
	if tp.offset != 0 {
		t.Errorf("scrollBy with few items: offset = %d, want 0", tp.offset)
	}
}

func TestClampOffset_ToleratesCursorMinusOne(t *testing.T) {
	tp := makeTreePanel(50, 10)
	tp.cursor = -1
	tp.offset = 5
	tp.clampOffset()
	if tp.cursor != -1 {
		t.Errorf("clampOffset changed cursor=-1 to %d", tp.cursor)
	}
	if tp.offset != 5 {
		t.Errorf("clampOffset moved offset for cursor=-1: %d, want 5", tp.offset)
	}
}

func TestNewTreePanel_EmptyTree_CursorOnAddNote(t *testing.T) {
	tp := newTreePanel(nil, 20, 10)
	if tp.cursor != -1 {
		t.Errorf("empty tree: cursor = %d, want -1", tp.cursor)
	}
}

func TestNewTreePanel_NonEmpty_CursorOnFirstFile(t *testing.T) {
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: []*TreeNode{
		{Name: "a.md", IsDir: false},
	}}
	tp := newTreePanel(root, 20, 10)
	if tp.cursor != 0 {
		t.Errorf("non-empty tree: cursor = %d, want 0", tp.cursor)
	}
}

func TestRebuildItems_EmptyAfterRebuild_ResetsCursorToMinusOne(t *testing.T) {
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: []*TreeNode{
		{Name: "a.md", IsDir: false},
	}}
	tp := newTreePanel(root, 20, 10)
	tp.cursor = 0
	tp.root.Children = nil
	tp.rebuildItems()
	if tp.cursor != -1 {
		t.Errorf("after rebuild to empty: cursor = %d, want -1", tp.cursor)
	}
}

func TestMoveUp_FromFirstFile_LandsOnAddNote(t *testing.T) {
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: []*TreeNode{
		{Name: "a.md", IsDir: false},
	}}
	tp := newTreePanel(root, 20, 10)
	tp.cursor = 0
	tp.moveUp()
	if tp.cursor != -1 {
		t.Errorf("moveUp from cursor=0: cursor = %d, want -1", tp.cursor)
	}
}

func TestMoveUp_FromAddNote_NoOp(t *testing.T) {
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: []*TreeNode{
		{Name: "a.md", IsDir: false},
	}}
	tp := newTreePanel(root, 20, 10)
	tp.cursor = -1
	tp.moveUp()
	if tp.cursor != -1 {
		t.Errorf("moveUp from cursor=-1: cursor = %d, want -1", tp.cursor)
	}
}

func TestMoveDown_FromAddNote_LandsOnFirstFile(t *testing.T) {
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: []*TreeNode{
		{Name: "a.md", IsDir: false},
	}}
	tp := newTreePanel(root, 20, 10)
	tp.cursor = -1
	tp.moveDown()
	if tp.cursor != 0 {
		t.Errorf("moveDown from cursor=-1: cursor = %d, want 0", tp.cursor)
	}
}

func TestMoveDown_FromAddNote_EmptyTree_StaysAtMinusOne(t *testing.T) {
	tp := newTreePanel(nil, 20, 10)
	tp.cursor = -1
	tp.moveDown()
	if tp.cursor != -1 {
		t.Errorf("moveDown empty tree: cursor = %d, want -1", tp.cursor)
	}
}

func TestOnAddNote(t *testing.T) {
	tp := newTreePanel(nil, 20, 10)
	if !tp.onAddNote() {
		t.Error("onAddNote() = false on empty tree, want true")
	}
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: []*TreeNode{
		{Name: "a.md", IsDir: false},
	}}
	tp = newTreePanel(root, 20, 10)
	if tp.onAddNote() {
		t.Error("onAddNote() = true on non-empty tree with cursor=0, want false")
	}
}

func TestSelectedNode_OnAddNote_ReturnsNil(t *testing.T) {
	tp := newTreePanel(nil, 20, 10)
	if tp.selectedNode() != nil {
		t.Error("selectedNode() with cursor=-1 should return nil")
	}
}

func TestMoveDown_AfterScroll_SnapsViewToCursor(t *testing.T) {
	tp := makeTreePanel(50, 10)
	tp.cursor = 0
	tp.offset = 0
	tp.scrollBy(20)
	if tp.offset != 20 {
		t.Fatalf("setup: scrollBy(20) → offset=%d, want 20", tp.offset)
	}
	tp.moveDown()
	if tp.cursor != 1 {
		t.Errorf("cursor = %d, want 1", tp.cursor)
	}
	if tp.offset != 1 {
		t.Errorf("offset = %d after moveDown, want 1 (snapped to cursor)", tp.offset)
	}
}

func TestMoveUp_AfterScroll_SnapsViewToCursor(t *testing.T) {
	tp := makeTreePanel(50, 10)
	tp.cursor = 30
	tp.offset = 25
	tp.scrollBy(-25)
	if tp.offset != 0 {
		t.Fatalf("setup: scrollBy(-25) → offset=%d, want 0", tp.offset)
	}
	tp.moveUp()
	if tp.cursor != 29 {
		t.Errorf("cursor = %d, want 29", tp.cursor)
	}
	if tp.offset == 0 {
		t.Errorf("offset = %d, want > 0 (snapped to bring cursor 29 into view)", tp.offset)
	}
}

func TestView_RendersPinnedAddNoteRow(t *testing.T) {
	tp := newTreePanel(nil, 20, 10)
	out := tp.View(true)
	if !strings.Contains(out, "Add note") {
		t.Errorf("View() does not contain pinned 'Add note' row:\n%s", out)
	}
}

func TestView_PinnedRow_VisibleEvenWhenScrolled(t *testing.T) {
	root := &TreeNode{Name: "root", IsDir: true, Expanded: true, Children: nil}
	for i := 0; i < 30; i++ {
		root.Children = append(root.Children, &TreeNode{Name: "f.md", IsDir: false})
	}
	tp := newTreePanel(root, 20, 10)
	tp.scrollBy(20)
	out := tp.View(false)
	if !strings.Contains(out, "Add note") {
		t.Errorf("Add note not rendered after scrolling:\n%s", out)
	}
}

// buildPanel constructs a TreePanel directly from a list of (path, depth, isDir)
// triples so tests can exercise the cursor helpers without spinning up a real
// vault.
func buildPanel(items []struct {
	Path  string
	Depth int
	IsDir bool
}) TreePanel {
	tp := TreePanel{height: 100, width: 40}
	tp.items = make([]FlatItem, len(items))
	for i, it := range items {
		tp.items[i] = FlatItem{
			Node:  &TreeNode{Path: it.Path, Name: it.Path, IsDir: it.IsDir},
			Depth: it.Depth,
		}
	}
	return tp
}

func TestTreePanel_IndexOfPath(t *testing.T) {
	tp := buildPanel([]struct {
		Path  string
		Depth int
		IsDir bool
	}{
		{"/v/a", 0, true},
		{"/v/a/x.md", 1, false},
		{"/v/b.md", 0, false},
	})

	if got := tp.indexOfPath("/v/a/x.md"); got != 1 {
		t.Errorf("indexOfPath = %d, want 1", got)
	}
	if got := tp.indexOfPath("/v/missing"); got != -1 {
		t.Errorf("indexOfPath missing = %d, want -1", got)
	}
}

func TestTreePanel_HasFollowingSiblingAtSameDepth(t *testing.T) {
	tp := buildPanel([]struct {
		Path  string
		Depth int
		IsDir bool
	}{
		{"/v/foo", 0, true},
		{"/v/foo/a", 1, true},
		{"/v/foo/a/n.md", 2, false},
		{"/v/foo/b", 1, true},
		{"/v/bar", 0, true},
	})

	if !tp.hasFollowingSiblingAtSameDepth(0) {
		t.Error("foo/ should have a following sibling (bar/)")
	}
	if !tp.hasFollowingSiblingAtSameDepth(1) {
		t.Error("a/ should have a following sibling (b/) at depth=1")
	}
	if tp.hasFollowingSiblingAtSameDepth(3) {
		t.Error("b/ has no same-depth follower before depth drops below 1")
	}
	if tp.hasFollowingSiblingAtSameDepth(4) {
		t.Error("bar/ is the last item; no follower")
	}

	if tp.hasFollowingSiblingAtSameDepth(-1) {
		t.Error("negative idx must return false")
	}
	if tp.hasFollowingSiblingAtSameDepth(99) {
		t.Error("oob idx must return false")
	}
}
