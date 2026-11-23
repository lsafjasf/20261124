package hdr

// Frame is an immutable-by-convention linear-light image. Pixels are stored in
// row-major order.
type Frame struct {
	Width  int
	Height int
	Pixels []RGB
}

// NewFrame allocates a black frame of the requested size.
func NewFrame(width, height int) Frame {
	if width < 0 || height < 0 {
		panic("hdr: negative frame dimensions")
	}
	return Frame{Width: width, Height: height, Pixels: make([]RGB, width*height)}
}

func (f Frame) Index(x, y int) int {
	return y*f.Width + x
}

func (f Frame) Valid(x, y int) bool {
	return x >= 0 && x < f.Width && y >= 0 && y < f.Height
}

func (f Frame) At(x, y int) RGB {
	return f.Pixels[f.Index(x, y)]
}

func (f *Frame) Set(x, y int, c RGB) {
	f.Pixels[f.Index(x, y)] = c
}

func (f Frame) Clone() Frame {
	out := Frame{Width: f.Width, Height: f.Height, Pixels: make([]RGB, len(f.Pixels))}
	copy(out.Pixels, f.Pixels)
	return out
}

// CompressFrame applies Compress to every pixel and returns a fresh frame plus
// a report describing the corrections that were necessary. The input frame is
// not modified.
func CompressFrame(f Frame, p Preset) (Frame, FrameReport) {
	p = p.normalized()
	out := Frame{Width: f.Width, Height: f.Height, Pixels: make([]RGB, len(f.Pixels))}
	report := FrameReport{
		Width:  f.Width,
		Height: f.Height,
		Preset: p.Name,
		Pixels: len(f.Pixels),
	}
	reasons := make([]Reason, len(f.Pixels))

	for i, c := range f.Pixels {
		res := Compress(c, p)
		out.Pixels[i] = res.Color
		if res.NegativeRecovered {
			report.NegativeRecovered++
		}
		if res.HighlightCompressed {
			report.HighlightCompressed++
		}
		if res.Unrecoverable != "" {
			report.Unrecoverable++
			reasons[i] = res.Unrecoverable
			switch res.Unrecoverable {
			case ReasonNonFinite:
				report.NonFinite++
			case ReasonNegativeLuminance:
				report.NegativeLuminance++
			case ReasonHighlightDetail:
				report.HighlightDetailLost++
			}
		}
	}

	report.Regions = findRegions(reasons, f.Width, f.Height)
	return out, report
}

// findRegions groups 4-connected pixels that share the same loss reason into
// axis-aligned regions. Regions are emitted in top-to-bottom, left-to-right
// order so reports are deterministic.
func findRegions(reasons []Reason, width, height int) []Region {
	seen := make([]bool, len(reasons))
	regions := make([]Region, 0)
	queue := make([]int, 0, 64)

	for start, reason := range reasons {
		if reason == "" || seen[start] {
			continue
		}
		seen[start] = true
		queue = queue[:0]
		queue = append(queue, start)
		region := Region{
			X:      start % width,
			Y:      start / width,
			Width:  1,
			Height: 1,
			Pixels: 0,
			Reason: reason,
		}
		minX, minY := region.X, region.Y
		maxX, maxY := region.X, region.Y

		for len(queue) > 0 {
			idx := queue[0]
			queue = queue[1:]
			x, y := idx%width, idx/width
			region.Pixels++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			for _, n := range [4]int{idx - 1, idx + 1, idx - width, idx + width} {
				if n < 0 || n >= len(reasons) || seen[n] || reasons[n] != reason {
					continue
				}
				nx, ny := n%width, n/width
				if absInt(nx-x)+absInt(ny-y) != 1 {
					continue
				}
				seen[n] = true
				queue = append(queue, n)
			}
		}

		region.X = minX
		region.Y = minY
		region.Width = maxX - minX + 1
		region.Height = maxY - minY + 1
		regions = append(regions, region)
	}
	return regions
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
