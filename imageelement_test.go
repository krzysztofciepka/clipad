package main

import "testing"

func TestParseImageElement(t *testing.T) {
	cases := []struct {
		line   string
		target string
		ok     bool
	}{
		{"![](assets/img-2026-07-03-abc12345.png)", "assets/img-2026-07-03-abc12345.png", true},
		{"![alt text](assets/pic.jpeg)", "assets/pic.jpeg", true},
		{"  ![](assets/pic.png)  ", "assets/pic.png", true},
		{"![](assets/notes.md)", "", false},
		{"text before ![](assets/pic.png)", "", false},
		{"![](assets/pic.png) trailing text", "", false},
		{"not an image", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		target, ok := parseImageElement(c.line)
		if ok != c.ok || target != c.target {
			t.Errorf("parseImageElement(%q) = (%q, %v), want (%q, %v)", c.line, target, ok, c.target, c.ok)
		}
	}
}
