package chromasiting

// BT.601 limited-range-ish reference colors used by the synthetic samples.
var (
	redY, redCb, redCr    = 81.0, 90.0, 240.0
	blueY, blueCb, blueCr = 41.0, 240.0, 110.0
)

// Sample is a synthetic 4:4:4 test image with a known vertical red/blue
// edge. BoundaryX is the true edge position in luma pixel coordinates
// (pixel centers are integers).
type Sample struct {
	Name      string
	Img       Image444
	BoundaryX float64
}

// edgeImage builds a w x h image whose left side is red and right side is
// blue. boundaryX is the continuous edge position; a pixel is a coverage
// blend of red and blue, so half-integer boundaries produce a mixed pixel.
func edgeImage(name string, w, h int, boundaryX float64) Sample {
	img := Image444{Y: NewPlane(w, h), Cb: NewPlane(w, h), Cr: NewPlane(w, h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Pixel x covers [x-0.5, x+0.5]; fraction left of the edge is red.
			red := boundaryX - (float64(x) - 0.5)
			red = minF(maxF(red, 0), 1)
			img.Y.Set(x, y, red*redY+(1-red)*blueY)
			img.Cb.Set(x, y, red*redCb+(1-red)*blueCb)
			img.Cr.Set(x, y, red*redCr+(1-red)*blueCr)
		}
	}
	return Sample{Name: name, Img: img, BoundaryX: boundaryX}
}

// RedBlueBoundary returns an even-sized sample with the edge exactly between
// the two middle luma columns (pixel centers are integers, so the gap
// between columns w/2-1 and w/2 is at x = w/2 - 0.5).
func RedBlueBoundary(w, h int) Sample {
	return edgeImage("red-blue-boundary", w, h, float64(w)/2-0.5)
}

// OddSize returns a sample with odd width and height; the 4:2:0 chroma
// planes are ceil(w/2) x ceil(h/2) and the edge sits between columns.
func OddSize(w, h int) Sample {
	return edgeImage("odd-size", w, h, float64(w)/2)
}

// HalfPixelOffset returns a sample whose edge is shifted by half a luma
// pixel off the column grid: it cuts through the center of column w/2,
// which therefore becomes a 50% red/blue blend.
func HalfPixelOffset(w, h int) Sample {
	return edgeImage("half-pixel-offset", w, h, float64(w)/2)
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
