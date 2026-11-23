package vobsub

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var reviewBackground = color.NRGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}

// RasterImage renders a crop. A nil background keeps transparency; otherwise
// the bitmap is composited over bg.
func RasterImage(r *Raster, st State, pal [16]RGB, bg *color.NRGBA) *image.NRGBA {
	if r == nil {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, r.Width, r.Height))
	for y := 0; y < r.Height; y++ {
		for x := 0; x < r.Width; x++ {
			p := r.Pixels[y*r.Width+x] & 3
			c := pal[int(st.PaletteMap[p])&15]
			a := int(st.Alpha[p]) * 17
			if bg == nil {
				if a == 0 {
					continue
				}
				dst.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(a)})
				continue
			}
			dst.SetNRGBA(x, y, blend(c, a, *bg))
		}
	}
	return dst
}

// CueCrop returns the original sub-picture crop with transparency.
func CueCrop(cue *Cue) *image.NRGBA {
	if cue == nil {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	return RasterImage(cue.Raster, cue.State, cue.Palette, nil)
}

// FrameAt composites cues accepted by include at timeMS on a neutral review
// background. This mirrors source timing, not generated ASS timing.
func FrameAt(cues []*Cue, size image.Point, timeMS int64, include func(*Cue) bool) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			dst.SetNRGBA(x, y, reviewBackground)
		}
	}
	DrawCues(dst, cues, size, func(c *Cue) bool {
		return c.Migratable && c.StartMS <= timeMS && timeMS < c.EndMS &&
			(include == nil || include(c))
	})
	return dst
}

// SourceFrameAt composites every source cue active at timeMS, including cues
// that cannot be migrated.
func SourceFrameAt(cues []*Cue, size image.Point, timeMS int64) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			dst.SetNRGBA(x, y, reviewBackground)
		}
	}
	DrawCues(dst, cues, size, func(c *Cue) bool {
		return c.StartMS <= timeMS && timeMS < c.EndMS
	})
	return dst
}

// RasterAtFrame composites one decoded state at its declared rectangle.
func RasterAtFrame(r *Raster, st State, pal [16]RGB, size image.Point) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			dst.SetNRGBA(x, y, reviewBackground)
		}
	}
	if r == nil {
		return dst
	}
	for y := 0; y < r.Height; y++ {
		fy := st.Rect.Y + y
		if fy < 0 || fy >= size.Y {
			continue
		}
		for x := 0; x < r.Width; x++ {
			fx := st.Rect.X + x
			if fx < 0 || fx >= size.X {
				continue
			}
			p := r.Pixels[y*r.Width+x] & 3
			a := int(st.Alpha[p]) * 17
			if a == 0 {
				continue
			}
			c := pal[int(st.PaletteMap[p])&15]
			dst.SetNRGBA(fx, fy, blend(c, a, dst.NRGBAAt(fx, fy)))
		}
	}
	return dst
}

// DrawCues composites cues into dst using their decoded rectangles.
func DrawCues(dst *image.NRGBA, cues []*Cue, size image.Point, include func(*Cue) bool) {
	for _, cue := range cues {
		if include != nil && !include(cue) {
			continue
		}
		if cue.Raster == nil {
			continue
		}
		for y := 0; y < cue.Raster.Height; y++ {
			fy := cue.State.Rect.Y + y
			if fy < 0 || fy >= size.Y {
				continue
			}
			for x := 0; x < cue.Raster.Width; x++ {
				fx := cue.State.Rect.X + x
				if fx < 0 || fx >= size.X {
					continue
				}
				p := cue.Raster.Pixels[y*cue.Raster.Width+x] & 3
				a := int(cue.State.Alpha[p]) * 17
				if a == 0 {
					continue
				}
				idx := int(cue.State.PaletteMap[p]) & 15
				c := cue.Palette[idx]
				old := dst.NRGBAAt(fx, fy)
				dst.SetNRGBA(fx, fy, blend(c, a, old))
			}
		}
	}
}

func blend(src RGB, alpha int, dst color.NRGBA) color.NRGBA {
	if alpha >= 255 {
		return color.NRGBA{R: src.R, G: src.G, B: src.B, A: 255}
	}
	if alpha <= 0 {
		return dst
	}
	inv := 255 - alpha
	return color.NRGBA{
		R: uint8((int(src.R)*alpha + int(dst.R)*inv) / 255),
		G: uint8((int(src.G)*alpha + int(dst.G)*inv) / 255),
		B: uint8((int(src.B)*alpha + int(dst.B)*inv) / 255),
		A: 255,
	}
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}
