package main

import "testing"

func TestShortcutProviderURL(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{"blackbox", defaultBlackboxURL},
		{"openrouter", defaultOpenRouterURL},
		{"opencode", defaultOpenCodeURL},
		{"unknown", defaultBlackboxURL}, // unknown providers fall back to blackbox
		{"", defaultBlackboxURL},
	}
	for _, c := range cases {
		if got := shortcutProviderURL(c.provider); got != c.want {
			t.Errorf("shortcutProviderURL(%q) = %q, want %q", c.provider, got, c.want)
		}
	}
}
