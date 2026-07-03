package main

import "strings"

// detectKittyGraphics reports whether the terminal supports the kitty graphics
// protocol, based on environment hints. This is a heuristic that avoids a
// mid-startup terminal query race (see setDarkBackground in main.go).
func detectKittyGraphics(getenv func(string) string) bool {
	if getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	term := strings.ToLower(getenv("TERM"))
	if strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") || strings.Contains(term, "wezterm") {
		return true
	}
	prog := strings.ToLower(getenv("TERM_PROGRAM"))
	switch prog {
	case "ghostty", "wezterm", "kitty":
		return true
	}
	return false
}
