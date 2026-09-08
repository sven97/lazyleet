// Package termimg renders raster images inline in a terminal. It targets two
// backends:
//
//   - Kitty graphics protocol with Unicode placeholders (kitty, Ghostty,
//     recent WezTerm): the image is transmitted once and displayed via a grid
//     of placeholder characters, so a cell-based TUI renderer keeps correct row
//     accounting and scrolling/redraw don't corrupt the image.
//   - Truecolor half-block fallback (everything else): each cell is a "▀" with
//     a foreground/background colour = two stacked pixels. Pure text.
package termimg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// maxSide caps the longest edge of the re-encoded image so the Kitty transmit
// payload stays small (a LeetCode diagram → ~15–30 KB PNG). The transmit
// sequence is re-sent on every render while the image's first row is on screen,
// so this trades a little sharpness for scroll smoothness.
const maxSide = 512

// Protocol is the chosen rendering backend.
type Protocol int

const (
	ProtoNone   Protocol = iota // images disabled
	ProtoBlocks                 // truecolor half-block fallback
	ProtoKitty                  // Kitty graphics + Unicode placeholders
)

func (p Protocol) String() string {
	switch p {
	case ProtoKitty:
		return "kitty"
	case ProtoBlocks:
		return "blocks"
	default:
		return "off"
	}
}

// Detect picks a protocol from the environment. `override` is the config value
// ("auto" | "off" | "blocks" | "kitty"); the LAZYLEET_IMG env var wins over it.
func Detect(override string) Protocol {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("LAZYLEET_IMG")))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(override))
	}
	switch v {
	case "off", "none":
		return ProtoNone
	case "blocks", "ansi":
		return ProtoBlocks
	case "kitty":
		return ProtoKitty
	}
	// auto
	term := strings.ToLower(os.Getenv("TERM"))
	prog := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "",
		strings.Contains(term, "kitty"),
		strings.Contains(term, "ghostty"),
		prog == "ghostty",
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "",
		os.Getenv("WEZTERM_PANE") != "":
		return ProtoKitty
	default:
		return ProtoBlocks
	}
}

// Image is a decoded image ready to render.
type Image struct {
	img  image.Image
	w, h int
	id   uint32 // stable per content, for the Kitty image id
	png  []byte // re-encoded PNG (Kitty transmit payload)
}

// Decode decodes PNG/JPEG/GIF/WebP bytes.
func Decode(data []byte) (*Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	img = downscale(img, maxSide)

	b := img.Bounds()
	h := fnv.New32a()
	_, _ = h.Write(data)
	id := h.Sum32()
	if id == 0 {
		id = 1
	}
	id &= 0x00FFFFFF // keep it in 24 bits (single foreground colour)

	im := &Image{img: img, w: b.Dx(), h: b.Dy(), id: id}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	im.png = buf.Bytes()
	return im, nil
}

// Render returns a string to embed in a view and the number of terminal rows it
// occupies. cols is the target width in cells; the height follows the aspect
// ratio (assuming cells are ~twice as tall as wide).
func (im *Image) Render(proto Protocol, cols int) (body string, rows int) {
	if im == nil || cols < 1 {
		return "", 0
	}
	if cols > 200 {
		cols = 200
	}
	rows = im.rowsFor(cols)
	switch proto {
	case ProtoKitty:
		return im.kitty(cols, rows), rows
	case ProtoBlocks:
		return im.blocks(cols, rows), rows
	default:
		return "", 0
	}
}

func (im *Image) rowsFor(cols int) int {
	// cell aspect ≈ 2.1 (height/width)
	r := int(float64(cols) * float64(im.h) / float64(im.w) / 2.1)
	if r < 1 {
		r = 1
	}
	if r > 40 {
		r = 40
	}
	return r
}

// --- fetch + disk cache -------------------------------------------------------

// Fetch returns the bytes for url, using an on-disk cache under dir. Only http/
// https URLs are fetched; the caller is responsible for host allow-listing.
func Fetch(ctx context.Context, dir, url string) ([]byte, error) {
	sum := sha256.Sum256([]byte(url))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+cacheExt(url))
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lazyleet")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(path, data, 0o644)
	return data, nil
}

func downscale(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	scale := float64(max) / float64(w)
	if float64(h)*scale > float64(max) {
		scale = float64(max) / float64(h)
	}
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func cacheExt(url string) string {
	if i := strings.LastIndexByte(url, '.'); i >= 0 && len(url)-i <= 6 {
		ext := url[i:]
		if j := strings.IndexAny(ext, "?#"); j >= 0 {
			ext = ext[:j]
		}
		return ext
	}
	return ".img"
}
