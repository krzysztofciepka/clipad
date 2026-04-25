package main

import (
	"strings"
	"testing"
)

func TestPickAsset_ExactMatch(t *testing.T) {
	rel := release{
		TagName: "v0.0.22",
		Assets: []releaseAsset{
			{Name: "clipad-v0.0.22-linux-amd64", BrowserDownloadURL: "https://example.test/a"},
		},
	}
	a, err := pickAsset(rel, "linux", "amd64")
	if err != nil {
		t.Fatalf("pickAsset: %v", err)
	}
	if a.Name != "clipad-v0.0.22-linux-amd64" {
		t.Fatalf("got %q", a.Name)
	}
}

func TestPickAsset_PicksMatchingFromMany(t *testing.T) {
	rel := release{
		TagName: "v0.0.22",
		Assets: []releaseAsset{
			{Name: "clipad-v0.0.22-darwin-arm64"},
			{Name: "clipad-v0.0.22-linux-amd64", BrowserDownloadURL: "https://example.test/want"},
			{Name: "checksums.txt"},
		},
	}
	a, err := pickAsset(rel, "linux", "amd64")
	if err != nil {
		t.Fatalf("pickAsset: %v", err)
	}
	if a.BrowserDownloadURL != "https://example.test/want" {
		t.Fatalf("picked wrong asset: %+v", a)
	}
}

func TestPickAsset_NoMatchReturnsError(t *testing.T) {
	rel := release{
		TagName: "v0.0.22",
		Assets:  []releaseAsset{{Name: "clipad-v0.0.22-darwin-arm64"}},
	}
	_, err := pickAsset(rel, "linux", "amd64")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "clipad-v0.0.22-linux-amd64") {
		t.Fatalf("error should mention expected asset name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "v0.0.22") {
		t.Fatalf("error should mention release tag, got: %v", err)
	}
}
