package fixture

import "math"

// renderRGB paints the shared timeline frame n as full-range RGB. The picture
// carries structural identity (timecode digits, scene bands, moving marker)
// that both SDR and HDR encodings keep even though tone differs.
func renderRGB(spec Spec, n int) [][][3]float64 {
	w, h := spec.Width, spec.Height
	img := make([][][3]float64, h)
	scene := n / 24
	for y := 0; y < h; y++ {
		row := make([][3]float64, w)
		for x := 0; x < w; x++ {
			var r, g, b float64
			fx := float64(x) / float64(w-1)
			fy := float64(y) / float64(h-1)
			switch scene {
			case 0:
				r, g, b = 0.10+0.55*fx, 0.15+0.35*fy, 0.55-0.35*fx
			case 1:
				r, g, b = 0.60-0.30*fy, 0.25+0.45*fx, 0.20+0.40*fy
			case 2:
				r, g, b = 0.20+0.30*fy, 0.55-0.25*fx, 0.55-0.30*fy
			default:
				r, g, b = 0.75-0.45*fx, 0.70-0.40*fy, 0.65-0.35*fx
			}
			// Flat middle band: a strong structural edge shared by both grades.
			if y >= h/2-4 && y <= h/2+4 {
				r, g, b = 0.92, 0.90, 0.86
			}
			if y >= h/2+8 && y <= h/2+16 {
				r, g, b = 0.05, 0.06, 0.08
			}
			// Moving marker, one pixel per frame, identical geometry per grade.
			mx := (n * 3) % w
			if y >= 6 && y <= 14 && x >= mx && x <= mx+6 {
				r, g, b = 0.95, 0.10, 0.12
			}
			row[x] = [3]float64{clamp01(r), clamp01(g), clamp01(b)}
		}
		img[y] = row
	}
	drawTimecode(img, n, 6, 20)
	drawSubtitleBar(img, spec, n)
	return img
}

func drawSubtitleBar(img [][][3]float64, spec Spec, n int) {
	var text string
	for _, sub := range spec.Subtitles() {
		if int64(n) >= sub.Start && int64(n) <= sub.End {
			text = sub.Text
		}
	}
	if text == "" {
		return
	}
	h := len(img)
	// Opaque lower-third bar plus the text as glyph holes: its left and right
	// edges are measurable per frame even without a full text renderer.
	y0 := h - 22
	for y := y0; y < h-6; y++ {
		for x := 10; x < spec.Width-10; x++ {
			img[y][x] = [3]float64{0.02, 0.02, 0.02}
		}
	}
	drawText(img, text, 16, y0+4, [3]float64{1, 1, 1})
}

var glyph = map[rune][]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "010", "010", "010"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	':': {"000", "010", "000", "010", "000"},
	' ': {"000", "000", "000", "000", "000"},
}

func drawTimecode(img [][][3]float64, n int, x0, y0 int) {
	total := n
	hh := total / (24 * 3600)
	mm := (total / (24 * 60)) % 60
	ss := (total / 24) % 60
	ff := total % 24
	text := format2(hh) + ":" + format2(mm) + ":" + format2(ss) + ":" + format2(ff)
	drawText(img, text, x0, y0, [3]float64{1, 1, 1})
}

func drawText(img [][][3]float64, text string, x0, y0 int, color [3]float64) {
	x := x0
	for _, ch := range text {
		rows, ok := glyph[ch]
		if !ok {
			rows = glyph[' ']
		}
		for gy, line := range rows {
			for gx, c := range line {
				if c == '1' {
					px, py := x+gx, y0+gy
					if py >= 0 && py < len(img) && px >= 0 && px < len(img[0]) {
						img[py][px] = color
					}
				}
			}
		}
		x += 4
	}
}

func format2(v int) string {
	return string(rune('0'+(v/10)%10)) + string(rune('0'+v%10))
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
