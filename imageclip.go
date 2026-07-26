package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

// clipEnvForPaste is an indirection over currentClipEnv so tests can inject
// a fixed environment for paste dispatch.
var clipEnvForPaste = currentClipEnv

var runCapture = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var runWithStdin = func(stdin []byte, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	return cmd.Run()
}

// lookPath is an indirection over exec.LookPath so tests can inject a fixed
// result without depending on real PATH contents.
var lookPath = exec.LookPath

func imageToolAvailable(env clipEnv) bool {
	name, _ := readImageCmd(env)
	if name == "" {
		return false
	}
	_, err := lookPath(name)
	return err == nil
}

func clipboardHasImage(env clipEnv) bool {
	name, args := probeImageCmd(env)
	if name == "" {
		return false
	}
	out, err := runCapture(name, args...)
	if err != nil {
		return false
	}
	return probeIndicatesImage(env, out)
}

func readClipboardImage(env clipEnv) ([]byte, error) {
	name, args := readImageCmd(env)
	if name == "" {
		return nil, fmt.Errorf("no clipboard tool for this environment")
	}
	return runCapture(name, args...)
}

func writeClipboardImage(env clipEnv, data []byte) error {
	name, args := writeImageCmd(env)
	if name == "" {
		return fmt.Errorf("no clipboard tool for this environment")
	}
	return runWithStdin(data, name, args...)
}
