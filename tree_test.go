package main

import "testing"

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
