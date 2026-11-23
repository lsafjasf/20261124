package audit

import (
	"fmt"
	"math/big"
	"strings"
)

// SVG renders a dependency-free error chart. float64 is used only for layout
// coordinates; all timestamps in the report were computed with rationals.
func (r *Report) SVG() string {
	const width, height = 960, 460
	const left, right, top, bottom = 76, 28, 78, 58
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)

	lastFrame := r.Project.DurationFrames()
	durationSec := 0.0
	if lastFrame > 0 {
		duration := new(big.Rat).Quo(new(big.Rat).SetInt64(lastFrame), r.Project.VideoRate.Rat())
		durationSec, _ = duration.Float64()
	}
	minErr, maxErr := int64(0), int64(0)
	for _, pt := range r.Curve {
		if pt.ErrorMS < minErr {
			minErr = pt.ErrorMS
		}
		if pt.ErrorMS > maxErr {
			maxErr = pt.ErrorMS
		}
	}
	if maxErr == 0 {
		maxErr = 1
	}
	if minErr == 0 {
		minErr = -1
	}
	pad := float64(maxErr-minErr) * 0.08
	yMin := float64(minErr) - pad
	yMax := float64(maxErr) + pad
	if yMax-yMin < 2 {
		yMax += 1
		yMin -= 1
	}

	x := func(frame int64) float64 {
		if lastFrame == 0 {
			return float64(left)
		}
		return float64(left) + float64(frame)/float64(lastFrame)*plotW
	}
	y := func(errMS int64) float64 {
		return float64(top) + (yMax-float64(errMS))/(yMax-yMin)*plotH
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n", width, height, width, height)
	fmt.Fprintf(&b, `<title>字幕时基漂移误差曲线</title>`+"\n")
	fmt.Fprintf(&b, `<desc>旧管线相对精确 24000/1001 帧率的字幕时间误差，片尾为 %d ms。</desc>`+"\n", r.MaxAbsErrorMS)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#0b1220"/>`+"\n")
	fmt.Fprintf(&b, `<text x="%d" y="30" fill="#e5edf8" font-family="sans-serif" font-size="18" font-weight="700">Subtitle timebase drift: 24000/1001 frames read as 24 fps</text>`+"\n", left)
	fmt.Fprintf(&b, `<text x="%d" y="50" fill="#8fa3bd" font-family="monospace" font-size="11">legacy e(t) = -t/1001 s · first error: %s · max |e| = %d ms</text>`+"\n",
		left, r.FirstErrorText(), r.MaxAbsErrorMS)

	// Plot frame and grid.
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%.1f" height="%.1f" fill="#111c2e" stroke="#2b3b55"/>`+"\n",
		left, top, plotW, plotH)
	for i := 0; i <= 5; i++ {
		frac := float64(i) / 5
		gx := float64(left) + frac*plotW
		sec := durationSec * frac
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" stroke="#1e2c43"/>`+"\n",
			gx, top, gx, top+int(plotH))
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" text-anchor="middle" fill="#7185a0" font-family="monospace" font-size="10">%.0f</text>`+"\n",
			gx, height-28, sec)
	}
	for _, tick := range []int64{0, minErr / 2, minErr} {
		gy := y(tick)
		fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#1e2c43"/>`+"\n",
			left, gy, left+int(plotW), gy)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" text-anchor="end" fill="#7185a0" font-family="monospace" font-size="10">%+d</text>`+"\n",
			left-10, gy+4, tick)
	}
	fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#46617f" stroke-width="1.5"/>`+"\n",
		left, y(0), left+int(plotW), y(0))
	fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#8fa3bd" font-family="monospace" font-size="11">error ms</text>`+"\n", left, top-14)
	fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="end" fill="#8fa3bd" font-family="monospace" font-size="11">exact seconds</text>`+"\n",
		width-right, height-10)

	// Curve.
	fmt.Fprintf(&b, `<polyline fill="none" stroke="#ff647c" stroke-width="2.2" points="`)
	for i, pt := range r.Curve {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%.2f,%.2f", x(pt.Frame), y(pt.ErrorMS))
	}
	fmt.Fprintf(&b, `"/>`+"\n")

	if r.FirstError != nil {
		fe := r.FirstError
		fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="4" fill="#fbbf24"/>`+"\n", x(fe.Frame), y(fe.ErrorMS))
		fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" fill="#fbbf24" font-family="monospace" font-size="10">first %+d ms</text>`+"\n",
			x(fe.Frame)+8, y(fe.ErrorMS)-8, fe.ErrorMS)
	}
	if len(r.Curve) > 0 {
		last := r.Curve[len(r.Curve)-1]
		fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="4" fill="#ff647c"/>`+"\n", x(last.Frame), y(last.ErrorMS))
		fmt.Fprintf(&b, `<text x="%.2f" y="%.2f" text-anchor="end" fill="#ff8ca0" font-family="monospace" font-size="10">end %+d ms</text>`+"\n",
			x(last.Frame)-8, y(last.ErrorMS)+16, last.ErrorMS)
	}
	fmt.Fprintf(&b, `</svg>`+"\n")
	return b.String()
}
