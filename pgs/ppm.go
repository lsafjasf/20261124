package pgs

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
)

// BackgroundFunc returns the backdrop color at an output pixel.
type BackgroundFunc func(x, y int) color.NRGBA

// SolidBackground returns a constant backdrop.
func SolidBackground(c color.NRGBA) BackgroundFunc {
	return func(int, int) color.NRGBA { return c }
}

// CheckerBackground returns a checkerboard backdrop, useful for inspecting
// transparency in a format that has no alpha channel.
func CheckerBackground(light, dark color.NRGBA, size int) BackgroundFunc {
	if size <= 0 {
		size = 8
	}
	return func(x, y int) color.NRGBA {
		if (x/size+y/size)%2 == 0 {
			return light
		}
		return dark
	}
}

// WritePPM writes a binary P6 PPM. The premultiplied subtitle layer is
// composited over background using source-over alpha blending.
func WritePPM(w io.Writer, img *image.RGBA, background color.NRGBA) error {
	return WritePPMFunc(w, img, SolidBackground(background))
}

// WritePPMFunc is WritePPM with a per-pixel backdrop.
func WritePPMFunc(w io.Writer, img *image.RGBA, background BackgroundFunc) error {
	if img == nil {
		return fmt.Errorf("pgs: nil image")
	}
	if background == nil {
		background = SolidBackground(color.NRGBA{A: 255})
	}
	bw := bufio.NewWriter(w)
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if _, err := fmt.Fprintf(bw, "P6\n%d %d\n255\n", width, height); err != nil {
		return err
	}
	row := make([]byte, width*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			off := y*img.Stride + x*4
			a := int(img.Pix[off+3])
			inv := 255 - a
			bg := background(x, y)
			row[x*3+0] = uint8(min(255, int(img.Pix[off+0])+int(bg.R)*inv/255))
			row[x*3+1] = uint8(min(255, int(img.Pix[off+1])+int(bg.G)*inv/255))
			row[x*3+2] = uint8(min(255, int(img.Pix[off+2])+int(bg.B)*inv/255))
		}
		if _, err := bw.Write(row); err != nil {
			return err
		}
	}
	return bw.Flush()
}
