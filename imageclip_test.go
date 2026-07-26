package main

import "testing"

func TestDetectClipEnv(t *testing.T) {
	way := func(k string) string { if k == "WAYLAND_DISPLAY" { return "wayland-0" }; return "" }
	x11 := func(k string) string { if k == "DISPLAY" { return ":0" }; return "" }
	none := func(string) string { return "" }
	if detectClipEnv(way) != clipWayland {
		t.Error("want wayland")
	}
	if detectClipEnv(x11) != clipX11 {
		t.Error("want x11")
	}
	if detectClipEnv(none) != clipNone {
		t.Error("want none")
	}
}

func TestClipCommands(t *testing.T) {
	n, a := readImageCmd(clipWayland)
	if n != "wl-paste" || a[0] != "--type" || a[1] != "image/png" {
		t.Errorf("wayland read = %q %v", n, a)
	}
	n, a = readImageCmd(clipX11)
	if n != "xclip" || a[len(a)-1] != "image/png" {
		t.Errorf("x11 read = %q %v", n, a)
	}
	n, _ = writeImageCmd(clipWayland)
	if n != "wl-copy" {
		t.Errorf("wayland write = %q", n)
	}
}

func TestProbeIndicatesImage(t *testing.T) {
	if !probeIndicatesImage(clipWayland, []byte("text/plain\nimage/png\n")) {
		t.Error("wayland: want image detected")
	}
	if probeIndicatesImage(clipWayland, []byte("text/plain\n")) {
		t.Error("wayland: want no image")
	}
	if !probeIndicatesImage(clipX11, []byte("TARGETS\nimage/png\nUTF8_STRING\n")) {
		t.Error("x11: want image detected")
	}
}

func TestClipboardHasImageUsesRunner(t *testing.T) {
	old := runCapture
	defer func() { runCapture = old }()
	runCapture = func(name string, args ...string) ([]byte, error) {
		return []byte("text/plain\nimage/png\n"), nil
	}
	if !clipboardHasImage(clipWayland) {
		t.Error("want image detected via injected runner")
	}
	runCapture = func(name string, args ...string) ([]byte, error) {
		return []byte("text/plain\n"), nil
	}
	if clipboardHasImage(clipWayland) {
		t.Error("want no image")
	}
}

func TestWriteClipboardImageUsesStdin(t *testing.T) {
	old := runWithStdin
	defer func() { runWithStdin = old }()
	var gotStdin []byte
	var gotName string
	runWithStdin = func(stdin []byte, name string, args ...string) error {
		gotStdin = stdin
		gotName = name
		return nil
	}
	_ = writeClipboardImage(clipWayland, []byte("BYTES"))
	if gotName != "wl-copy" || string(gotStdin) != "BYTES" {
		t.Errorf("write = %q stdin=%q", gotName, gotStdin)
	}
}
