package compare

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"hdrthumb/hdr"
)

// ToRGBA converts a linear-light frame to an 8-bit sRGB image.
func ToRGBA(f hdr.Frame) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			c := f.At(x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: encodeSRGB(c.R),
				G: encodeSRGB(c.G),
				B: encodeSRGB(c.B),
				A: 255,
			})
		}
	}
	return img
}

// WritePNG writes a frame as an 8-bit sRGB PNG.
func WritePNG(path string, f hdr.Frame) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, ToRGBA(f))
}

// WriteStrip writes frames left-to-right with a dark separator. It is used to
// put the naive clamp and the three presets into one inspectable image.
func WriteStrip(path string, frames []hdr.Frame, gap int) error {
	if len(frames) == 0 {
		return nil
	}
	width, height := frames[0].Width, frames[0].Height
	for _, f := range frames[1:] {
		if f.Width != width || f.Height != height {
			return fmt.Errorf("compare: frame dimensions differ: %dx%d", f.Width, f.Height)
		}
	}
	if gap < 0 {
		gap = 0
	}
	stripW := width*len(frames) + gap*(len(frames)-1)
	strip := image.NewRGBA(image.Rect(0, 0, stripW, height))
	draw.Draw(strip, strip.Bounds(), &image.Uniform{C: color.RGBA{16, 16, 16, 255}}, image.Point{}, draw.Src)
	for i, f := range frames {
		panel := ToRGBA(f)
		x := i * (width + gap)
		draw.Draw(strip, image.Rect(x, 0, x+width, height), panel, image.Point{}, draw.Src)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, strip)
}

func encodeSRGB(v float64) uint8 {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	var s float64
	if v <= 0.0031308 {
		s = 12.92 * v
	} else {
		s = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	if s < 0 {
		s = 0
	}
	if s > 1 {
		s = 1
	}
	return uint8(math.Round(s * 255))
}
