package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
)

// RenderPNG draws the subtitle bitmap composited over a checkerboard
// background so transparency stays visible. No fonts, no players.
func RenderPNG(w io.Writer, sub *Subtitle, palette *[16][3]uint8) error {
	img := image.NewRGBA(image.Rect(0, 0, sub.Width, sub.Height))
	for y := 0; y < sub.Height; y++ {
		for x := 0; x < sub.Width; x++ {
			slot := sub.Pixels[y*sub.Width+x]
			a := sub.Alpha[slot]
			var bg uint8 = 0x40
			if (x/4+y/4)%2 == 0 {
				bg = 0x70
			}
			c := palette[sub.Colors[slot]]
			alpha16 := uint32(a)
			inv := 255 - alpha16
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((uint32(c[0])*alpha16 + uint32(bg)*inv) / 255),
				G: uint8((uint32(c[1])*alpha16 + uint32(bg)*inv) / 255),
				B: uint8((uint32(c[2])*alpha16 + uint32(bg)*inv) / 255),
				A: 255,
			})
		}
	}
	return png.Encode(w, img)
}

// ASCIIArt renders the bitmap as terminal text: two pixel rows per
// character row, ' '=transparent, '.'/'+'/'#' = color slots 1..3.
func ASCIIArt(sub *Subtitle) string {
	shades := []byte{' ', '.', '+', '#'}
	var b strings.Builder
	for y := 0; y < sub.Height; y += 2 {
		for x := 0; x < sub.Width; x++ {
			top := sub.Pixels[y*sub.Width+x]
			bot := uint8(0)
			if y+1 < sub.Height {
				bot = sub.Pixels[(y+1)*sub.Width+x]
			}
			ch := shades[top]
			if top == 0 && bot != 0 {
				ch = ','
			}
			b.WriteByte(ch)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Describe returns a one-line summary of a decoded subtitle.
func Describe(sub *Subtitle) string {
	start, end := sub.ByteRange()
	return fmt.Sprintf("#%03d [%s lang=%d(%s)] bytes 0x%09X-0x%09X (%d B) %dx%d@(%d,%d)",
		sub.Entry.Index, sub.Entry.Timestamp, sub.Entry.LangIdx, sub.Entry.LangID,
		start, end, end-start, sub.Width, sub.Height, sub.X, sub.Y)
}
