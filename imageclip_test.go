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
