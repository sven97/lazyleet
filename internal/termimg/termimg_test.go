package termimg

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDetectOverrides(t *testing.T) {
	t.Setenv("LAZYLEET_IMG", "")
	if got := Detect("off"); got != ProtoNone {
		t.Errorf("override off -> %v", got)
	}
	if got := Detect("blocks"); got != ProtoBlocks {
		t.Errorf("override blocks -> %v", got)
	}
	t.Setenv("LAZYLEET_IMG", "kitty")
	if got := Detect("off"); got != ProtoKitty {
		t.Errorf("env should win over config: %v", got)
	}
}

func TestDetectAutoKitty(t *testing.T) {
	t.Setenv("LAZYLEET_IMG", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_PANE", "")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "xterm-ghostty")
	if got := Detect("auto"); got != ProtoKitty {
		t.Errorf("TERM=xterm-ghostty -> %v, want kitty", got)
	}
	t.Setenv("TERM", "xterm-256color")
	if got := Detect("auto"); got != ProtoBlocks {
		t.Errorf("plain xterm -> %v, want blocks", got)
	}
}

func TestDecodeDownscales(t *testing.T) {
	im, err := Decode(testPNG(t, 2000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	if im.w > maxSide || im.h > maxSide {
		t.Fatalf("not downscaled: %dx%d", im.w, im.h)
	}
	if im.w != maxSide { // longest side hits the cap
		t.Errorf("width = %d, want %d", im.w, maxSide)
	}
}

func TestRenderBlocks(t *testing.T) {
	im, _ := Decode(testPNG(t, 40, 40))
	body, rows := im.Render(ProtoBlocks, 20)
	if rows < 1 {
		t.Fatal("no rows")
	}
	if lines := strings.Count(body, "\n") + 1; lines != rows {
		t.Errorf("body has %d lines, rows=%d", lines, rows)
	}
	if !strings.Contains(body, "▀") { // ▀
		t.Error("block output missing the half-block glyph")
	}
	if !strings.Contains(body, "\x1b[38;2;") || !strings.Contains(body, "48;2;") {
		t.Error("block output missing truecolor fg/bg")
	}
}

func TestRenderKitty(t *testing.T) {
	im, _ := Decode(testPNG(t, 300, 150))
	body, rows := im.Render(ProtoKitty, 40)
	if rows < 1 {
		t.Fatal("no rows")
	}
	if !strings.HasPrefix(body, "\x1b_Ga=t,f=100,q=2,i=") {
		t.Errorf("kitty output should start with a transmit sequence, got %.40q", body)
	}
	if !strings.Contains(body, placeholder) {
		t.Error("kitty output missing the Unicode placeholder")
	}
	if !strings.Contains(body, "\x1b[38;2;") {
		t.Error("kitty output missing the id-carrying foreground colour")
	}
	if lines := strings.Count(body, "\n") + 1; lines != rows {
		t.Errorf("kitty body has %d lines, rows=%d", lines, rows)
	}
}

func TestKittyChunking(t *testing.T) {
	im, _ := Decode(testPNG(t, 600, 600)) // big enough to need multiple chunks
	seq := kittyTransmit(im.id, im.png)
	// every payload between ';' and ESC\ must be <= 4096 chars
	for _, part := range strings.Split(seq, "\x1b\\") {
		if part == "" {
			continue
		}
		semi := strings.IndexByte(part, ';')
		if semi < 0 {
			t.Fatalf("chunk without a ';': %.30q", part)
		}
		if payload := part[semi+1:]; len(payload) > 4096 {
			t.Fatalf("chunk payload is %d chars, > 4096", len(payload))
		}
	}
}
