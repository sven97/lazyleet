package termimg

import (
	"encoding/base64"
	"strings"
)

// placeholder is Kitty's Unicode placeholder code point (U+10EEEE).
const placeholder = "\U0010EEEE"

// rowColumnDiacritics is the start of Kitty's canonical row/column diacritic
// list (rowcolumn-diacritics.txt). Index i -> the combining mark for row/column
// i. We only need indices up to the image's row count (capped at 40).
var rowColumnDiacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F, 0x0346, 0x034A,
	0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364, 0x0365,
	0x0366, 0x0367, 0x0368, 0x0369, 0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F,
	0x0483, 0x0484, 0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
	0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1, 0x05A8, 0x05A9,
	0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611, 0x0612, 0x0613, 0x0614, 0x0615,
	0x0616, 0x0617, 0x0657, 0x0658,
}

func diacritic(i int) rune {
	if i < 0 {
		i = 0
	}
	if i >= len(rowColumnDiacritics) {
		i = len(rowColumnDiacritics) - 1
	}
	return rowColumnDiacritics[i]
}

// kittyPlaceholders renders rows of Unicode placeholder cells (no transmit).
// The image id is carried in the 24-bit foreground colour of the cells; the
// first cell of each row carries explicit row/column diacritics and the rest of
// the row auto-increments the column.
func (im *Image) kittyPlaceholders(cols, rows int) string {
	var b strings.Builder
	b.Grow(cols*rows*4 + rows*16)

	fg := kittyFG(im.id)
	const reset = "\x1b[39m"

	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fg)
		b.WriteString(placeholder)
		b.WriteRune(diacritic(r))
		b.WriteRune(diacritic(0))
		for c := 1; c < cols; c++ {
			b.WriteString(placeholder)
		}
		b.WriteString(reset)
	}
	return b.String()
}

func kittyFG(id uint32) string {
	return "\x1b[38;2;" +
		itoa(int((id>>16)&0xFF)) + ";" +
		itoa(int((id>>8)&0xFF)) + ";" +
		itoa(int(id&0xFF)) + "m"
}

// kittyTransmit builds the "transmit only" escape sequence (a=t) carrying the
// PNG payload, chunked at 4096 base64 chars, with responses suppressed (q=2).
func kittyTransmit(id uint32, png []byte) string {
	enc := base64.StdEncoding.EncodeToString(png)
	const chunk = 4096

	var b strings.Builder
	if len(enc) <= chunk {
		b.WriteString("\x1b_Ga=t,f=100,q=2,i=")
		b.WriteString(itoa(int(id)))
		b.WriteByte(';')
		b.WriteString(enc)
		b.WriteString("\x1b\\")
		return b.String()
	}
	for i := 0; i < len(enc); i += chunk {
		end := i + chunk
		if end > len(enc) {
			end = len(enc)
		}
		more := 1
		if end == len(enc) {
			more = 0
		}
		if i == 0 {
			b.WriteString("\x1b_Ga=t,f=100,q=2,i=")
			b.WriteString(itoa(int(id)))
			b.WriteString(",m=")
			b.WriteString(itoa(more))
			b.WriteByte(';')
		} else {
			b.WriteString("\x1b_Gm=")
			b.WriteString(itoa(more))
			b.WriteByte(';')
		}
		b.WriteString(enc[i:end])
		b.WriteString("\x1b\\")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
