// Command regen (re)creates the regression samples under testdata/:
// sRGB source images plus their expected Rec.709 outputs produced by
// the fixed pipeline. Run it whenever the pipeline intentionally
// changes, then review the diff.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	c "github.com/lsafjasf/20261124/colorxform"
	"github.com/lsafjasf/20261124/colorxform/buggy"
)

const outDir = "testdata"

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	must(err)
	defer f.Close()
	must(png.Encode(f, img))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "regen:", err)
		os.Exit(1)
	}
}

// darkGradient16 is a 16-bit sRGB gradient covering the shadow range
// where double-gamma handling is most visible.
func darkGradient16() *image.NRGBA64 {
	const w, h = 256, 64
	img := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		v := uint16(uint32(x) * 65535 / 255 / 10) // 0..0.1 of full scale
		for y := 0; y < h; y++ {
			img.SetNRGBA64(x, y, color.NRGBA64{v, v, v, 0xffff})
		}
	}
	return img
}

// grayRamp8 is a full-range 8-bit sRGB gray ramp.
func grayRamp8() *image.NRGBA {
	const w, h = 256, 64
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(x), uint8(x), 0xff})
		}
	}
	return img
}

// saturatedPatches16 holds saturated sRGB color patches that push the
// ACEScg intermediate toward its gamut boundary.
func saturatedPatches16() *image.NRGBA64 {
	patches := [][3]float64{
		{1, 0, 0}, {0, 1, 0}, {0, 0, 1},
		{0, 1, 1}, {1, 0, 1}, {1, 1, 0},
		{0.75, 0.1, 0.4}, {0.05, 0.6, 0.9},
	}
	const pw, h = 32, 64
	img := image.NewNRGBA64(image.Rect(0, 0, pw*len(patches), h))
	for i, p := range patches {
		c64 := color.NRGBA64{
			uint16(p[0] * 65535), uint16(p[1] * 65535), uint16(p[2] * 65535), 0xffff,
		}
		for x := i * pw; x < (i+1)*pw; x++ {
			for y := 0; y < h; y++ {
				img.SetNRGBA64(x, y, c64)
			}
		}
	}
	return img
}

func convert(src image.Image, depth int) image.Image {
	out, err := c.ConvertImage(src, c.Options{OutputDepth: depth, ClampOutput: true})
	must(err)
	return out
}

// convertBuggy renders a source through the historical defective
// pipeline (all three faults combined) for before/after comparison.
func convertBuggy(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			cr, cg, cb := buggy.Combined(
				float64(r)/65535, float64(g)/65535, float64(bl)/65535)
			clamp := func(v float64) uint8 {
				if v < 0 {
					v = 0
				}
				if v > 1 {
					v = 1
				}
				return uint8(v*255 + 0.5)
			}
			dst.SetNRGBA(x, y, color.NRGBA{clamp(cr), clamp(cg), clamp(cb), 0xff})
		}
	}
	return dst
}

func main() {
	must(os.MkdirAll(outDir, 0o755))
	type sample struct {
		name string
		src  image.Image
	}
	for _, s := range []sample{
		{"dark_gradient_16", darkGradient16()},
		{"gray_ramp_8", grayRamp8()},
		{"saturated_patches_16", saturatedPatches16()},
	} {
		srcPath := filepath.Join(outDir, s.name+"_srgb.png")
		writePNG(srcPath, s.src)
		writePNG(filepath.Join(outDir, s.name+"_before_buggy.png"), convertBuggy(s.src))
		for _, depth := range []int{8, 16} {
			outPath := filepath.Join(outDir,
				fmt.Sprintf("%s_expected_rec709_%d.png", s.name, depth))
			writePNG(outPath, convert(s.src, depth))
		}
		fmt.Println("wrote", srcPath, "and expected Rec.709 outputs (8/16-bit)")
	}
}
