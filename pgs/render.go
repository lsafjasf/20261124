package pgs

import (
	"fmt"
	"image"
	"math"
	"sort"
	"time"
)

const maxRenderDimension = 32768

// RenderAt renders the composition visible at the requested time at its
// native resolution.
func (s *Stream) RenderAt(at time.Duration) (*Frame, error) {
	return s.Render(RenderOptions{At: at})
}

// RenderAtPTS renders the composition visible at an exact 90 kHz timestamp.
func (s *Stream) RenderAtPTS(pts uint32) (*Frame, error) {
	return s.Render(RenderOptions{PTS: pts, UsePTS: true})
}

// Render renders the composition visible at opts.At and scales it when output
// dimensions are requested.
func (s *Stream) Render(opts RenderOptions) (*Frame, error) {
	var (
		d   *decoder
		err error
	)
	if opts.UsePTS {
		d, err = s.stateAtTicks(opts.PTS)
	} else {
		d, err = s.stateAt(opts.At)
	}
	if err != nil {
		return nil, err
	}
	comp := d.composition
	targetW, targetH, err := outputSize(comp.width, comp.height, opts.Width, opts.Height)
	if err != nil {
		return nil, err
	}

	src := image.NewRGBA(image.Rect(0, 0, comp.width, comp.height))
	canvas := Rect{X: 0, Y: 0, Width: comp.width, Height: comp.height}
	var palette *parsedPalette
	if len(comp.objects) > 0 {
		palette = d.palettes[comp.paletteID]
		if palette == nil {
			return nil, fmt.Errorf("pgs: composition references undefined palette %d", comp.paletteID)
		}
	}

	frame := &Frame{
		PTS:          comp.pts,
		Time:         ticksDuration(comp.pts),
		SourceWidth:  comp.width,
		SourceHeight: comp.height,
		Width:        targetW,
		Height:       targetH,
		PaletteID:    comp.paletteID,
	}

	windowIndex := make(map[uint8]int)
	for _, co := range comp.objects {
		obj := d.objects[co.id]
		if obj == nil || !obj.complete {
			return nil, fmt.Errorf("pgs: composition references undefined object %d", co.id)
		}
		window, ok := d.windows[co.windowID]
		if !ok {
			return nil, fmt.Errorf("pgs: composition references undefined window %d", co.windowID)
		}

		visible, sourceRect, err := drawObject(src, obj, co, window.rect, canvas, palette)
		if err != nil {
			return nil, err
		}
		info := ObjectInfo{
			ID:          co.id,
			WindowID:    co.windowID,
			Cropped:     co.cropped,
			Composition: Rect{X: co.x, Y: co.y, Width: sourceRect.Width, Height: sourceRect.Height},
			Source:      sourceRect,
			Clipped:     visible,
			Output:      scaleRect(visible, comp.width, comp.height, targetW, targetH),
		}
		frame.Objects = append(frame.Objects, info)

		idx, exists := windowIndex[co.windowID]
		if !exists {
			idx = len(frame.Windows)
			windowIndex[co.windowID] = idx
			clipped := Intersect(window.rect, canvas)
			frame.Windows = append(frame.Windows, WindowInfo{
				ID:            co.windowID,
				Source:        window.rect,
				Clipped:       clipped,
				Output:        scaleRect(window.rect, comp.width, comp.height, targetW, targetH),
				OutputClipped: scaleRect(clipped, comp.width, comp.height, targetW, targetH),
			})
		}
		frame.Windows[idx].ObjectIDs = append(frame.Windows[idx].ObjectIDs, co.id)
	}
	for id, window := range d.windows {
		if _, exists := windowIndex[id]; exists {
			continue
		}
		clipped := Intersect(window.rect, canvas)
		frame.Windows = append(frame.Windows, WindowInfo{
			ID:            id,
			Source:        window.rect,
			Clipped:       clipped,
			Output:        scaleRect(window.rect, comp.width, comp.height, targetW, targetH),
			OutputClipped: scaleRect(clipped, comp.width, comp.height, targetW, targetH),
		})
	}
	sort.Slice(frame.Windows, func(i, j int) bool { return frame.Windows[i].ID < frame.Windows[j].ID })

	if targetW != comp.width || targetH != comp.height {
		src = resample(src, targetW, targetH)
	}
	frame.Image = src
	return frame, nil
}

func outputSize(srcW, srcH, wantW, wantH int) (int, int, error) {
	if wantW < 0 || wantH < 0 {
		return 0, 0, fmt.Errorf("pgs: negative output size %dx%d", wantW, wantH)
	}
	w, h := wantW, wantH
	switch {
	case w == 0 && h == 0:
		w, h = srcW, srcH
	case w == 0:
		w = int(math.Round(float64(srcW) * float64(h) / float64(srcH)))
	case h == 0:
		h = int(math.Round(float64(srcH) * float64(w) / float64(srcW)))
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("pgs: invalid output size %dx%d", w, h)
	}
	if w > maxRenderDimension || h > maxRenderDimension || int64(w)*int64(h) > 1<<28 {
		return 0, 0, fmt.Errorf("pgs: output size %dx%d is too large", w, h)
	}
	return w, h, nil
}

func drawObject(dst *image.RGBA, obj *parsedObject, co compositionObject, window, canvas Rect, palette *parsedPalette) (Rect, Rect, error) {
	if obj.width <= 0 || obj.height <= 0 || len(obj.pixels) != obj.width*obj.height {
		return Rect{}, Rect{}, fmt.Errorf("pgs: object %d has invalid bitmap", obj.id)
	}
	full := Rect{Width: obj.width, Height: obj.height}
	source := full
	if co.cropped {
		source = co.crop
	}
	source = Intersect(source, full)
	if source.Empty() {
		return Rect{}, source, nil
	}

	dest := Rect{
		X:      co.x + source.X - (co.crop.X),
		Y:      co.y + source.Y - (co.crop.Y),
		Width:  source.Width,
		Height: source.Height,
	}
	if !co.cropped {
		dest.X, dest.Y = co.x, co.y
	}
	limit := Intersect(window, canvas)
	visible := Intersect(dest, limit)
	if visible.Empty() {
		return Rect{}, source, nil
	}

	for y := visible.Y; y < visible.Y+visible.Height; y++ {
		sy := source.Y + y - dest.Y
		for x := visible.X; x < visible.X+visible.Width; x++ {
			sx := source.X + x - dest.X
			index := obj.pixels[sy*obj.width+sx]
			if !palette.present[index] {
				continue
			}
			entry := palette.entries[index]
			if entry.a == 0 {
				continue
			}
			blendPremultiplied(dst, x, y, entry)
		}
	}
	return visible, source, nil
}

func blendPremultiplied(dst *image.RGBA, x, y int, src paletteEntry) {
	off := y*dst.Stride + x*4
	a := int(src.a)
	inv := 255 - a
	dst.Pix[off+0] = uint8((int(src.r)*a + int(dst.Pix[off+0])*inv + 127) / 255)
	dst.Pix[off+1] = uint8((int(src.g)*a + int(dst.Pix[off+1])*inv + 127) / 255)
	dst.Pix[off+2] = uint8((int(src.b)*a + int(dst.Pix[off+2])*inv + 127) / 255)
	dst.Pix[off+3] = uint8((a*255 + int(dst.Pix[off+3])*inv + 127) / 255)
}

func scaleRect(r Rect, srcW, srcH, dstW, dstH int) Rect {
	if r.Empty() {
		return Rect{}
	}
	x0 := scaleCoord(r.X, srcW, dstW)
	y0 := scaleCoord(r.Y, srcH, dstH)
	x1 := scaleCoord(r.X+r.Width, srcW, dstW)
	y1 := scaleCoord(r.Y+r.Height, srcH, dstH)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

func scaleCoord(v, src, dst int) int {
	return int(math.Round(float64(v) * float64(dst) / float64(src)))
}
