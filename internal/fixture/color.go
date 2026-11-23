package fixture

import "math"

// rgbToYUV encodes a full-range linear-ish RGB frame into limited-range
// 8-bit YUV444. grade selects the production color path:
//
//	"sdr": BT.709 matrix, BT.1886 (gamma 2.4) transfer, scene light as-is.
//	"hdr": BT.2020 matrix, SMPTE ST.2084 PQ transfer, 1.6x scene light.
//
// The two grades intentionally differ in tone while sharing geometry.
func rgbToYUV(img [][][3]float64, grade string) (y, u, v []byte, w, h int) {
	h = len(img)
	w = len(img[0])
	y = make([]byte, w*h)
	u = make([]byte, w*h)
	v = make([]byte, w*h)

	hdr := grade == "hdr"
	gain := 1.0
	if hdr {
		gain = 1.6
	}

	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			rgb := img[row][col]
			var rr, gg, bb float64
			if hdr {
				rr = pqEncode(clamp01(rgb[0] * gain))
				gg = pqEncode(clamp01(rgb[1] * gain))
				bb = pqEncode(clamp01(rgb[2] * gain))
			} else {
				rr = gammaEncode(rgb[0])
				gg = gammaEncode(rgb[1])
				bb = gammaEncode(rgb[2])
			}
			var fy, fu, fv float64
			if hdr {
				fy = 0.2627*rr + 0.6780*gg + 0.0593*bb
				fu = (bb - fy) / 1.8814
				fv = (rr - fy) / 1.4746
			} else {
				fy = 0.2126*rr + 0.7152*gg + 0.0722*bb
				fu = (bb - fy) / 1.8556
				fv = (rr - fy) / 1.5748
			}
			idx := row*w + col
			y[idx] = limitedLuma(fy)
			u[idx] = limitedChroma(fu)
			v[idx] = limitedChroma(fv)
		}
	}
	return y, u, v, w, h
}

func gammaEncode(linear float64) float64 {
	const a = 0.099
	const gamma = 2.4
	if linear < 0.018 {
		return 4.5 * linear
	}
	return (1+a)*math.Pow(linear, 1/gamma) - a
}

// pqEncode implements SMPTE ST.2084 (peak normalized to 10000 nit scale).
func pqEncode(linear float64) float64 {
	const m1 = 0.1593017578125
	const m2 = 78.84375
	const c1 = 0.8359375
	const c2 = 18.8515625
	const c3 = 18.6875
	if linear <= 0 {
		return 0
	}
	lp := math.Pow(linear, m1)
	num := c1 + c2*lp
	den := 1 + c3*lp
	return math.Pow(num/den, m2)
}

func limitedLuma(v float64) byte {
	return clamp8(math.Round(v*219 + 16))
}

func limitedChroma(v float64) byte {
	return clamp8(math.Round(v*224 + 128))
}

func clamp8(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}
