package chroma

import "fmt"

// Origin controls whether a block processor uses absolute image coordinates
// or coordinates local to each block. Global is required for seamless tiling.
type Origin uint8

const (
	GlobalOrigin Origin = iota
	BlockLocalOrigin
)

func (o Origin) String() string {
	if o == GlobalOrigin {
		return "global"
	}
	return "block-local"
}

// Options configures chroma upsampling.
type Options struct {
	Convention Convention
	Origin     Origin
	BlockW     int
	BlockH     int
}

// DefaultOptions are the corrected settings used by the experiment.
func DefaultOptions() Options {
	return Options{
		Convention: Centered,
		Origin:     GlobalOrigin,
		BlockW:     8,
		BlockH:     8,
	}
}

// BrokenOptions model the reported half-pixel coordinate defect.
func BrokenOptions() Options {
	return Options{
		Convention: LeftTop,
		Origin:     GlobalOrigin,
		BlockW:     8,
		BlockH:     8,
	}
}

// BlockLocalOptions isolate the common tiling defect where a block processor
// restarts at local coordinate zero instead of using absolute coordinates.
func BlockLocalOptions() Options {
	return Options{
		Convention: Centered,
		Origin:     BlockLocalOrigin,
		BlockW:     8,
		BlockH:     8,
	}
}

// UpsamplePlane expands one chroma plane with bilinear interpolation.
func UpsamplePlane(src *Plane, outW, outH int, opt Options) (*Plane, error) {
	if src == nil || src.W <= 0 || src.H <= 0 {
		return nil, fmt.Errorf("invalid source plane %v", src)
	}
	if outW <= 0 || outH <= 0 {
		return nil, fmt.Errorf("invalid output size %dx%d", outW, outH)
	}
	if opt.BlockW <= 0 || opt.BlockH <= 0 {
		return nil, fmt.Errorf("invalid block size %dx%d", opt.BlockW, opt.BlockH)
	}

	dst := NewPlane(outW, outH)
	ForEachBlock(outW, outH, opt.BlockW, opt.BlockH, func(block Block) {
		upsampleBlock(src, dst, block, opt)
	})
	return dst, nil
}

func upsampleBlock(src, dst *Plane, block Block, opt Options) {
	for y := block.Y0; y < block.Y1; y++ {
		cy := y
		if opt.Origin == BlockLocalOrigin {
			cy = y - block.Y0
		}
		ym := Axis(cy, src.H, opt.Convention)
		for x := block.X0; x < block.X1; x++ {
			cx := x
			if opt.Origin == BlockLocalOrigin {
				cx = x - block.X0
			}
			xm := Axis(cx, src.W, opt.Convention)
			top := src.At(xm.I0, ym.I0)*xm.W0 + src.At(xm.I1, ym.I0)*xm.W1
			bottom := src.At(xm.I0, ym.I1)*xm.W0 + src.At(xm.I1, ym.I1)*xm.W1
			dst.Set(x, y, top*ym.W0+bottom*ym.W1)
		}
	}
}

// Block is a half-open destination rectangle.
type Block struct {
	X0 int
	Y0 int
	X1 int
	Y1 int
}

// ForEachBlock visits the clipped tile rectangles covering an image.
func ForEachBlock(w, h, blockW, blockH int, fn func(Block)) {
	for y0 := 0; y0 < h; y0 += blockH {
		for x0 := 0; x0 < w; x0 += blockW {
			y1 := min(y0+blockH, h)
			x1 := min(x0+blockW, w)
			fn(Block{X0: x0, Y0: y0, X1: x1, Y1: y1})
		}
	}
}

// Subsample420 averages Cb and Cr over each 2x2 luma block. Odd rows and
// columns are handled by averaging the samples that exist; no padding is added.
func Subsample420(src *YUVImage) (cb, cr *Plane, err error) {
	if src == nil || src.W <= 0 || src.H <= 0 {
		return nil, nil, fmt.Errorf("invalid YUV image")
	}
	cw, ch := (src.W+1)/2, (src.H+1)/2
	cb, cr = NewPlane(cw, ch), NewPlane(cw, ch)
	for by := 0; by < ch; by++ {
		y0 := 2 * by
		y1 := min(y0+2, src.H)
		for bx := 0; bx < cw; bx++ {
			x0 := 2 * bx
			x1 := min(x0+2, src.W)
			var sumCb, sumCr float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					p := src.At(x, y)
					sumCb += p.Cb
					sumCr += p.Cr
				}
			}
			n := float64((x1 - x0) * (y1 - y0))
			cb.Set(bx, by, sumCb/n)
			cr.Set(bx, by, sumCr/n)
		}
	}
	return cb, cr, nil
}
