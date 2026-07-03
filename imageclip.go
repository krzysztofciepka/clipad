package main

import (
	"os"
	"strings"
)

type clipEnv int

const (
	clipNone clipEnv = iota
	clipWayland
	clipX11
)

func detectClipEnv(getenv func(string) string) clipEnv {
	if getenv("WAYLAND_DISPLAY") != "" {
		return clipWayland
	}
	if getenv("DISPLAY") != "" {
		return clipX11
	}
	return clipNone
}

func probeImageCmd(env clipEnv) (string, []string) {
	switch env {
	case clipWayland:
		return "wl-paste", []string{"--list-types"}
	case clipX11:
		return "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}
	}
	return "", nil
}

func readImageCmd(env clipEnv) (string, []string) {
	switch env {
	case clipWayland:
		return "wl-paste", []string{"--type", "image/png", "--no-newline"}
	case clipX11:
		return "xclip", []string{"-selection", "clipboard", "-o", "-t", "image/png"}
	}
	return "", nil
}

func writeImageCmd(env clipEnv) (string, []string) {
	switch env {
	case clipWayland:
		return "wl-copy", []string{"--type", "image/png"}
	case clipX11:
		return "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}
	}
	return "", nil
}

func probeIndicatesImage(env clipEnv, out []byte) bool {
	return strings.Contains(string(out), "image/png")
}

// currentClipEnv is a convenience wrapper over the real environment.
func currentClipEnv() clipEnv { return detectClipEnv(os.Getenv) }
