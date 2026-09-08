package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/sven97/lazyleet/internal/termimg"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 15), uint8(y * 20), 200, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestImageURLs(t *testing.T) {
	md := "text ![a diagram](https://assets.leetcode.com/uploads/x.png) more " +
		"![](https://assets.leetcode.com/uploads/y.jpg) and a dup " +
		"![z](https://assets.leetcode.com/uploads/x.png)"
	got := imageURLs(md)
	if len(got) != 2 {
		t.Fatalf("got %d urls, want 2: %v", len(got), got)
	}
	if got[0] != "https://assets.leetcode.com/uploads/x.png" {
		t.Errorf("first url = %q", got[0])
	}
}

func TestRenderStatementFallbackWhenNoImages(t *testing.T) {
	md := "# Title\n\nSome text ![tree](https://assets.leetcode.com/uploads/t.png) end."
	out := renderStatementMD(nil, md, 60, nil)
	if !strings.Contains(out, "⟨tree⟩") {
		t.Errorf("expected ⟨tree⟩ placeholder, got:\n%s", out)
	}
	if strings.Contains(out, "assets.leetcode.com") {
		t.Errorf("raw image markdown leaked:\n%s", out)
	}
}

func TestRenderStatementInlinesKnownImage(t *testing.T) {
	md := "before ![g](https://assets.leetcode.com/uploads/g.png) after"
	im, err := termimg.Decode(tinyPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	imgs := &statementImages{
		proto: termimg.ProtoBlocks,
		byURL: map[string]*termimg.Image{"https://assets.leetcode.com/uploads/g.png": im},
	}
	out := renderStatementMD(nil, md, 60, imgs)
	if strings.Contains(out, "⟨g⟩") {
		t.Errorf("known image should not fall back to text:\n%s", out)
	}
	if !strings.Contains(out, "▀") { // block glyph from the rendered image
		t.Errorf("expected an inline block image, got:\n%s", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Errorf("surrounding text dropped:\n%s", out)
	}
}
