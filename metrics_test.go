package main

import "testing"

func TestComputeMetricsPlainProse(t *testing.T) {
	words, chars := computeMetrics("The quick brown fox")
	if words != 4 {
		t.Errorf("words = %d, want 4", words)
	}
	if chars != 19 {
		t.Errorf("chars = %d, want 19", chars)
	}
}

func TestComputeMetricsStripsCodeFence(t *testing.T) {
	text := "hello world\n```\ncode here ignored\nmore code\n```\nafter fence"
	words, _ := computeMetrics(text)
	if words != 4 { // "hello world" + "after fence"; fenced content excluded
		t.Errorf("words = %d, want 4", words)
	}
}

func TestComputeMetricsFenceOnly(t *testing.T) {
	words, _ := computeMetrics("```\ncode\n```")
	if words != 0 {
		t.Errorf("words = %d, want 0", words)
	}
}

func TestStripCodeFencesUnterminated(t *testing.T) {
	got := stripCodeFences("prose line\n```\nunclosed code\nstill code")
	if got != "prose line" {
		t.Errorf("stripCodeFences = %q, want %q", got, "prose line")
	}
}

func TestComputeMetricsMultiByte(t *testing.T) {
	words, chars := computeMetrics("café über señor")
	if words != 3 {
		t.Errorf("words = %d, want 3", words)
	}
	if chars != 15 { // rune count, not byte count
		t.Errorf("chars = %d, want 15", chars)
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t\n"} {
		words, _ := computeMetrics(in)
		if words != 0 {
			t.Errorf("computeMetrics(%q) words = %d, want 0", in, words)
		}
	}
}

func TestReadingMinutes(t *testing.T) {
	tests := []struct {
		words int
		want  int
	}{
		{0, 0}, {1, 1}, {220, 1}, {221, 2}, {440, 2}, {441, 3},
	}
	for _, tt := range tests {
		if got := readingMinutes(tt.words); got != tt.want {
			t.Errorf("readingMinutes(%d) = %d, want %d", tt.words, got, tt.want)
		}
	}
}
