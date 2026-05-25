package main

import (
	"reflect"
	"testing"
)

func TestMetricTokensEditMode(t *testing.T) {
	sb := StatusBar{editorFocused: true, bufferText: "one two three"}
	prefix, tokens := sb.metricTokens()
	if prefix != "" {
		t.Errorf("prefix = %q, want \"\"", prefix)
	}
	want := []string{"3 words", "13 chars"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestMetricTokensPreviewMode(t *testing.T) {
	sb := StatusBar{editorFocused: true, previewMode: true, bufferText: "one two three"}
	_, tokens := sb.metricTokens()
	want := []string{"~1m read"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestMetricTokensSelection(t *testing.T) {
	sb := StatusBar{editorFocused: true, selectionActive: true, selectionText: "alpha beta"}
	prefix, tokens := sb.metricTokens()
	if prefix != "sel: " {
		t.Errorf("prefix = %q, want \"sel: \"", prefix)
	}
	want := []string{"2 words", "10 chars"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestMetricTokensHiddenWhenUnfocused(t *testing.T) {
	sb := StatusBar{editorFocused: false, bufferText: "lots of words here"}
	_, tokens := sb.metricTokens()
	if tokens != nil {
		t.Errorf("tokens = %v, want nil", tokens)
	}
}

func TestMetricTokensHiddenWhenEmpty(t *testing.T) {
	sb := StatusBar{editorFocused: true, bufferText: "   \n  "}
	_, tokens := sb.metricTokens()
	if tokens != nil {
		t.Errorf("tokens = %v, want nil", tokens)
	}
}

func TestMetricTokensSelectionWhitespaceHidden(t *testing.T) {
	sb := StatusBar{editorFocused: true, selectionActive: true, selectionText: "   \n  "}
	_, tokens := sb.metricTokens()
	if tokens != nil {
		t.Errorf("tokens = %v, want nil", tokens)
	}
}

func TestMetricTokensSelectionBeatsPreview(t *testing.T) {
	sb := StatusBar{editorFocused: true, previewMode: true, selectionActive: true, selectionText: "hello world"}
	prefix, tokens := sb.metricTokens()
	if prefix != "sel: " {
		t.Errorf("prefix = %q, want \"sel: \"", prefix)
	}
	want := []string{"2 words", "11 chars"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestFitMetricsDropsTokens(t *testing.T) {
	tokens := []string{"3 words", "13 chars"} // "3 words · 13 chars" == 18 wide
	if got := fitMetrics("", tokens, 100); got != "3 words · 13 chars" {
		t.Errorf("budget 100: got %q, want %q", got, "3 words · 13 chars")
	}
	if got := fitMetrics("", tokens, 10); got != "3 words" {
		t.Errorf("budget 10: got %q, want %q", got, "3 words")
	}
	if got := fitMetrics("", tokens, 3); got != "" {
		t.Errorf("budget 3: got %q, want \"\"", got)
	}
}
