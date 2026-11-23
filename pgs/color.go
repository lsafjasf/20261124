package pgs

import "math"

// ycbcrToRGBA converts an HDMV palette entry to straight RGBA. Blu-ray uses
// limited-range BT.709 for HD graphics and BT.601 for SD graphics.
func ycbcrToRGBA(y, cr, cb, alpha uint8, videoHeight int) paletteEntry {
	yf := float64(y)
	crf := float64(cr) - 128
	cbf := float64(cb) - 128

	luma := 1.164383 * (yf - 16)
	var r, g, b float64
	if videoHeight > 0 && videoHeight <= 576 {
		r = luma + 1.596027*crf
		g = luma - 0.391762*cbf - 0.812968*crf
		b = luma + 2.017232*cbf
	} else {
		r = luma + 1.792741*crf
		g = luma - 0.213249*cbf - 0.532909*crf
		b = luma + 2.112402*cbf
	}

	clamp := func(v float64) uint8 {
		v = math.Round(v)
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}

	return paletteEntry{r: clamp(r), g: clamp(g), b: clamp(b), a: alpha}
}
