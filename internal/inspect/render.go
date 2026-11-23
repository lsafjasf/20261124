package inspect

import (
	"image"
	"image/color"
	"image/png"
	"io"

	"vxcheck/internal/vx"
)

// WritePNG renders one frame's luma plane as a grayscale PNG. Luma is shown
// directly (limited range expanded) so SDR and HDR geometry can be compared
// without applying either transfer function.
func WritePNG(w io.Writer, f vx.Frame, width, height int) error {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for i, y := range f.Y {
		v := int(y)
		if v < 16 {
			v = 16
		}
		if v > 235 {
			v = 235
		}
		img.Pix[i] = uint8((v - 16) * 255 / 219)
	}
	return png.Encode(w, img)
}

// WriteSideBySide renders the SDR and HDR frame next to each other in one PNG
// with a 4-pixel divider, each half labeled implicitly by position (left SDR,
// right HDR).
func WriteSideBySide(w io.Writer, sdr, hdr vx.Frame, width, height int) error {
	const divider = 4
	canvas := image.NewGray(image.Rect(0, 0, 2*width+divider, height))
	paint(canvas, sdr, width, height, 0)
	for y := 0; y < height; y++ {
		for x := 0; x < divider; x++ {
			canvas.SetGray(width+x, y, color.Gray{Y: 128})
		}
	}
	paint(canvas, hdr, width, height, width+divider)
	return png.Encode(w, canvas)
}

func paint(dst *image.Gray, f vx.Frame, width, height, offX int) {
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := int(f.Y[y*width+x])
			if v < 16 {
				v = 16
			}
			if v > 235 {
				v = 235
			}
			dst.SetGray(offX+x, y, color.Gray{Y: uint8((v - 16) * 255 / 219)})
		}
	}
}
