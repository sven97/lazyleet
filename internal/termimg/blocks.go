package termimg

import (
	"image"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// blocks renders the image with the truecolor half-block technique: one "▀" per
// cell, its foreground = the upper pixel and background = the lower pixel, so a
// cell shows two vertically stacked pixels.
func (im *Image) blocks(cols, rows int) string {
	pxW, pxH := cols, rows*2
	dst := image.NewRGBA(image.Rect(0, 0, pxW, pxH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), im.img, im.img.Bounds(), xdraw.Over, nil)

	// dark backdrop for transparent pixels
	const bgR, bgG, bgB = 0x1a, 0x1a, 0x1a

	at := func(x, y int) (r, g, b uint8) {
		c := dst.RGBAAt(x, y)
		a := float64(c.A) / 255
		return uint8(float64(c.R)*a + bgR*(1-a)),
			uint8(float64(c.G)*a + bgG*(1-a)),
			uint8(float64(c.B)*a + bgB*(1-a))
	}

	var sb strings.Builder
	sb.Grow(cols*rows*22 + rows)
	for row := 0; row < rows; row++ {
		if row > 0 {
			sb.WriteByte('\n')
		}
		for col := 0; col < cols; col++ {
			tr, tg, tb := at(col, row*2)
			br, bg, bb := at(col, row*2+1)
			sb.WriteString("\x1b[38;2;")
			sb.WriteString(itoa(int(tr)))
			sb.WriteByte(';')
			sb.WriteString(itoa(int(tg)))
			sb.WriteByte(';')
			sb.WriteString(itoa(int(tb)))
			sb.WriteString(";48;2;")
			sb.WriteString(itoa(int(br)))
			sb.WriteByte(';')
			sb.WriteString(itoa(int(bg)))
			sb.WriteByte(';')
			sb.WriteString(itoa(int(bb)))
			sb.WriteByte('m')
			sb.WriteString("▀") // ▀
		}
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}
