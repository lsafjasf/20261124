package main

import (
	"fmt"
	"image/color"
	"math"
)

// renderChart draws the full test chart: PQ curve with samples and
// out-of-range shading on top, three peak-decoded ramps below.
func renderChart(samples []float64) *canvas {
	img := newCanvas(1240, 1010, colBg)
	drawHeader(img)
	drawCurvePlot(img, samples)
	drawRamps(img)
	drawFooter(img)
	return img
}

func drawHeader(img *canvas) {
	drawText(img, 90, 18, "pq st2084 pipeline: linear f64 > pq f64 > quant 10bit code > decode f64 > peak clip", colText)
	drawText(img, 90, 40, "float kept end to end. the only quantizers: 10bit pq code, and 8bit display out", colDim)
	drawText(img, 90, 62, "pq is absolute 0-10000nit: peak switch never changes a code, so mid gray stays put", colDim)
}

func drawCurvePlot(img *canvas, samples []float64) {
	// Out-of-range shading per peak (beyond 1000 / 2000 / 4000 nit).
	for x := plotX0; x <= plotX1; x++ {
		n := nitsOfX(x)
		switch {
		case n > 4000:
			img.blendRect(x, plotY0, 1, plotY1-plotY0, colRed, 0.45)
		case n > 2000:
			img.blendRect(x, plotY0, 1, plotY1-plotY0, colOrange, 0.22)
		case n > 1000:
			img.blendRect(x, plotY0, 1, plotY1-plotY0, colRed, 0.16)
		}
	}
	// Grid: decades on x, quarters on y.
	for d := int(logMin); d <= int(logMax); d++ {
		x := xOfNits(math.Pow(10, float64(d)))
		img.vline(x, plotY0, plotY1, colGrid)
		label := fmt.Sprintf("%g", math.Pow(10, float64(d)))
		if d == int(logMax) {
			label += "nit"
		}
		lx := x - textWidth(label)/2
		if lx < plotX0-30 {
			lx = plotX0 - 30
		}
		if lx+textWidth(label) > plotX1+4 {
			lx = plotX1 + 4 - textWidth(label)
		}
		drawText(img, lx, plotY1+8, label, colDim)
	}
	for _, q := range []float64{0.25, 0.5, 0.75, 1.0} {
		y := yOfPQ(q)
		img.hline(plotX0, plotX1, y, colGrid)
		drawText(img, plotX0-46, y-7, fmt.Sprintf("%.2f", q), colDim)
	}
	drawText(img, plotX0-46, plotY0-20, "pq", colDim)
	// Axes.
	img.vline(plotX0, plotY0, plotY1, colAxis)
	img.hline(plotX0, plotX1, plotY1, colAxis)
	// PQ curve.
	for x := plotX0; x <= plotX1; x++ {
		y := yOfPQ(PQEncode(nitsOfX(x)))
		img.fillRect(x, y-1, 1, 3, colCurve)
	}
	// Sampled points on the curve.
	for _, n := range samples {
		img.dot(xOfNits(n), yOfPQ(PQEncode(n)), 3, colSample)
	}
	// Peak markers.
	for i, peak := range DisplayPeaks {
		x := xOfNits(peak)
		img.vline(x, plotY0, plotY1, peakColors[i])
		drawText(img, x+5, plotY0+6+18*i, fmt.Sprintf("%.0fnit", peak), peakColors[i])
	}
	// Mid-gray marker.
	xm := xOfNits(MidGrayNits)
	img.vline(xm, plotY0, plotY1, colMid)
	drawText(img, xm+5, plotY1-40, fmt.Sprintf("mid gray %.1fnit", MidGrayNits), colMid)
	drawText(img, xm+5, plotY1-22, fmt.Sprintf("code=%d", EncodePQ10(MidGrayNits)), colMid)
	// Legend.
	img.dot(plotX0+16, plotY0+16, 3, colSample)
	drawText(img, plotX0+28, plotY0+9, "curve samples", colDim)
	img.fillRect(plotX0+210, plotY0+11, 14, 10, colRed)
	drawText(img, plotX0+232, plotY0+9, "out of range > peak", colDim)
}

// Ramp bar geometry: x maps the 10-bit PQ code 0..1023 linearly.
const (
	rampX0, rampX1 = 90, 1200
	rampY0         = 560
	rampH          = 52
	rampGap        = 78
)

func codeOfX(x int) uint16 {
	c := (x - rampX0) * PQCodeMax / (rampX1 - rampX0)
	if c < 0 {
		return 0
	}
	if c > PQCodeMax {
		return PQCodeMax
	}
	return uint16(c)
}

func xOfCode(code uint16) int {
	return rampX0 + int(code)*(rampX1-rampX0)/PQCodeMax
}

// drawRamps draws one bar per display peak: every PQ code decoded and
// mapped to 8-bit gray; codes beyond the peak are painted red (clipped
// out-of-range values). A white line marks the mid-gray code, which sits
// at the same x in every bar.
func drawRamps(img *canvas) {
	midCode := EncodePQ10(MidGrayNits)
	for i, peak := range DisplayPeaks {
		y := rampY0 + i*rampGap
		drawText(img, rampX0, y-22, fmt.Sprintf("peak %.0fnit: pq code > decode f64 > clip > 8bit", peak), peakColors[i])
		for x := rampX0; x <= rampX1; x++ {
			code := codeOfX(x)
			nits := DecodePQ10(code)
			level, clipped := DisplayMap(nits, peak)
			var col color.RGBA
			if clipped {
				col = colRed
			} else {
				col = color.RGBA{level, level, level, 255}
			}
			img.fillRect(x, y, 1, rampH, col)
		}
		// Frame and clip-boundary label.
		img.vline(rampX0-1, y-1, y+rampH+1, colAxis)
		img.vline(rampX1+1, y-1, y+rampH+1, colAxis)
		img.hline(rampX0-1, rampX1+2, y-1, colAxis)
		img.hline(rampX0-1, rampX1+2, y+rampH, colAxis)
		xc := xOfCode(EncodePQ10(peak))
		img.vline(xc, y-6, y, peakColors[i])
		clipLabel := fmt.Sprintf("clip >%.0f", peak)
		drawText(img, xc-textWidth(clipLabel)-6, y+rampH-16, clipLabel, peakColors[i])
	}
	// Mid-gray code line across all three bars: same x everywhere.
	xm := xOfCode(midCode)
	for i := range DisplayPeaks {
		y := rampY0 + i*rampGap
		img.vline(xm, y-4, y+rampH+4, colMid)
	}
	drawText(img, xm+6, rampY0+2*rampGap+rampH+12,
		fmt.Sprintf("mid gray %.1fnit code=%d (same x in all bars)", MidGrayNits, midCode), colMid)
	drawText(img, rampX0, rampY0+2*rampGap+rampH+12, "x axis: 10bit pq code 0..1023", colDim)
}

func drawFooter(img *canvas) {
	y := 940
	drawText(img, 90, y, "quant starts here: pq signal f64 > round(pq*1023) = 10bit code", colText)
	drawText(img, 90, y+20, "still float: scene nits in, pq signal, decoded nits. display 8bit is output-only", colText)
	drawText(img, 90, y+40, "red = out of range (nits > peak), clipped to 255. codes left of red are untouched", colText)
}
