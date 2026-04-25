package main

import (
	"fmt"
)

const (
	repoOwner = "krzysztofciepka"
	repoName  = "clipad"
)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func pickAsset(rel release, goos, goarch string) (releaseAsset, error) {
	want := fmt.Sprintf("clipad-%s-%s-%s", rel.TagName, goos, goarch)
	for _, a := range rel.Assets {
		if a.Name == want {
			return a, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("no asset matching %s in release %s", want, rel.TagName)
}
