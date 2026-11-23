package pgs

import (
	"image"
	"math"
)

func resample(src *image.RGBA, dstW, dstH int) *image.RGBA {
	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	if dstW <= srcW && dstH <= srcH {
		resampleBox(src, dst)
	} else {
		resampleBilinear(src, dst)
	}
	return dst
}

func resampleBox(src, dst *image.RGBA) {
	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	dstW, dstH := dst.Bounds().Dx(), dst.Bounds().Dy()

	for y := 0; y < dstH; y++ {
		y0 := y * srcH / dstH
		y1 := (y + 1) * srcH / dstH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dstW; x++ {
			x0 := x * srcW / dstW
			x1 := (x + 1) * srcW / dstW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sum [4]int64
			var count int64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					off := sy*src.Stride + sx*4
					sum[0] += int64(src.Pix[off+0])
					sum[1] += int64(src.Pix[off+1])
					sum[2] += int64(src.Pix[off+2])
					sum[3] += int64(src.Pix[off+3])
					count++
				}
			}
			out := y*dst.Stride + x*4
			dst.Pix[out+0] = uint8((sum[0] + count/2) / count)
			dst.Pix[out+1] = uint8((sum[1] + count/2) / count)
			dst.Pix[out+2] = uint8((sum[2] + count/2) / count)
			dst.Pix[out+3] = uint8((sum[3] + count/2) / count)
		}
	}
}

func resampleBilinear(src, dst *image.RGBA) {
	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	dstW, dstH := dst.Bounds().Dx(), dst.Bounds().Dy()

	for y := 0; y < dstH; y++ {
		fy := (float64(y)+0.5)*float64(srcH)/float64(dstH) - 0.5
		if fy < 0 {
			fy = 0
		}
		y0 := int(math.Floor(fy))
		if y0 >= srcH-1 {
			y0 = srcH - 1
		}
		y1 := min(y0+1, srcH-1)
		wy := fy - float64(y0)
		for x := 0; x < dstW; x++ {
			fx := (float64(x)+0.5)*float64(srcW)/float64(dstW) - 0.5
			if fx < 0 {
				fx = 0
			}
			x0 := int(math.Floor(fx))
			if x0 >= srcW-1 {
				x0 = srcW - 1
			}
			x1 := min(x0+1, srcW-1)
			wx := fx - float64(x0)

			c00 := src.Pix[y0*src.Stride+x0*4:]
			c10 := src.Pix[y0*src.Stride+x1*4:]
			c01 := src.Pix[y1*src.Stride+x0*4:]
			c11 := src.Pix[y1*src.Stride+x1*4:]
			out := y*dst.Stride + x*4
			for c := 0; c < 4; c++ {
				top := float64(c00[c])*(1-wx) + float64(c10[c])*wx
				bottom := float64(c01[c])*(1-wx) + float64(c11[c])*wx
				dst.Pix[out+c] = uint8(math.Round(top*(1-wy) + bottom*wy))
			}
		}
	}
}
