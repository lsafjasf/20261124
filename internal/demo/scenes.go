package demo

import (
	"fmt"
	"math"

	"github.com/lsafjasf/20261124/internal/master"
)

const (
	width     = 640
	height    = 360
	grayX     = 32
	grayY     = 32
	graySize  = 64
	subtitleX = 80
	subtitleY = 310
	subtitleW = 480
	subtitleH = 30
)

var subtitle = master.Rect{
	X: subtitleX, Y: subtitleY, Width: subtitleW, Height: subtitleH,
	Anchor: "bottom-center",
}

var middleGray = master.Rect{X: grayX, Y: grayY, Width: graySize, Height: graySize}

func nightFrame(shot string, frameNo int) (*master.Frame, *master.FrameAnalysis) {
	c := newCanvas(width, height, shot, frameNo, subtitle)
	c.fill(
		master.Vec3{0.0012, 0.0028, 0.0090},
		master.Vec3{0.0180, 0.0230, 0.0400},
	)
	phase := float64(frameNo) * 0.7
	for i := 0; i < 90; i++ {
		x := int(math.Mod(float64(i*137+19), width))
		y := 8 + int(math.Mod(float64(i*73+11), 190))
		twinkle := 0.05 + 0.10*(0.5+0.5*math.Sin(phase+float64(i)*1.7))
		c.set(x, y, master.Vec3{twinkle * 0.65, twinkle * 0.82, twinkle * 1.15})
	}
	moonX := 500.0 + float64(frameNo-1)*3
	c.circle(moonX, 76, 27, func(t float64) master.Vec3 {
		v := 0.7 + 1.8*smoothstep(0, 0.8, t)
		return master.Vec3{v * 0.78, v * 0.88, v}
	})

	buildings := []struct {
		x, y, w, h int
		v          master.Vec3
	}{
		{0, 250, 118, 110, master.Vec3{0.007, 0.008, 0.010}},
		{96, 218, 128, 142, master.Vec3{0.011, 0.012, 0.014}},
		{210, 236, 116, 124, master.Vec3{0.006, 0.007, 0.010}},
		{316, 205, 142, 155, master.Vec3{0.009, 0.010, 0.013}},
		{446, 232, 194, 128, master.Vec3{0.005, 0.006, 0.008}},
	}
	for _, b := range buildings {
		c.rect(b.x, b.y, b.w, b.h, b.v)
	}
	for i := 0; i < 52; i++ {
		x := 112 + (i%13)*15
		y := 232 + (i/13)*30
		if x > 300 && x < 340 {
			continue
		}
		level := 0.08 + float64((i*7)%5)*0.05
		c.rect(x, y, 7, 13, master.Vec3{level * 1.4, level * 0.80, level * 0.32})
	}

	levels := []float64{1.0, 2.0, 4.0, 8.0}
	highlights := make([]master.HighlightRegion, 0, 4)
	for i, level := range levels {
		x := 118 + i*29
		c.rect(x, 260, 17, 22, master.Vec3{level, level, level})
		highlights = append(highlights, master.HighlightRegion{
			Name: fmt.Sprintf("window_%d", i+1), ExpectedNits: level * 100,
			Rect: master.Rect{X: x, Y: 260, Width: 17, Height: 22},
		})
	}
	c.rect(grayX-5, grayY-5, graySize+10, graySize+10, master.Vec3{0.003, 0.003, 0.003})
	c.rect(grayX, grayY, graySize, graySize, gray18())
	return c.f, &master.FrameAnalysis{
		MiddleGray: &middleGray,
		Highlights: highlights,
	}
}

func windowFrame(shot string, frameNo int) (*master.Frame, *master.FrameAnalysis) {
	c := newCanvas(width, height, shot, frameNo, subtitle)
	c.fill(
		master.Vec3{0.085, 0.075, 0.062},
		master.Vec3{0.022, 0.019, 0.016},
	)
	c.rect(0, 286, width, height-286, master.Vec3{0.016, 0.012, 0.009})
	for y := 286; y < height; y += 18 {
		c.rect(0, y, width, 2, master.Vec3{0.024, 0.018, 0.013})
	}
	c.rect(330, 30, 286, 190, master.Vec3{0.019, 0.016, 0.013})
	c.rect(340, 40, 266, 170, master.Vec3{0.034, 0.030, 0.025})

	levels := []float64{1.0, 2.0, 4.0, 8.0}
	highlights := make([]master.HighlightRegion, 0, 4)
	tint := master.Vec3{0.95, 1.00, 1.12}
	for i, level := range levels {
		x := 352 + i*60
		cloudShift := (frameNo - 1) * 3
		c.rect(x, 54, 54, 136, tint.Mul(level))
		c.rect(x, 112+cloudShift, 54, 8, tint.Mul(level*0.72))
		c.rect(x, 170+cloudShift/2, 54, 5, tint.Mul(level*0.84))
		highlights = append(highlights, master.HighlightRegion{
			Name: fmt.Sprintf("outside_pane_%d", i+1), ExpectedNits: level * 100,
			Rect: master.Rect{X: x, Y: 54, Width: 54, Height: 136},
		})
	}
	c.rect(594, 40, 12, 170, master.Vec3{0.035, 0.030, 0.024})
	c.rect(250+frameNo*2, 226, 142, 60, master.Vec3{0.026, 0.020, 0.014})
	c.rect(270+frameNo*2, 222, 102, 5, master.Vec3{0.055, 0.042, 0.028})
	c.rect(grayX-5, grayY-5, graySize+10, graySize+10, master.Vec3{0.006, 0.005, 0.004})
	c.rect(grayX, grayY, graySize, graySize, gray18())
	return c.f, &master.FrameAnalysis{
		MiddleGray: &middleGray,
		Highlights: highlights,
	}
}

func skinFrame(shot string, frameNo int) (*master.Frame, *master.FrameAnalysis) {
	c := newCanvas(width, height, shot, frameNo, subtitle)
	c.fill(
		master.Vec3{0.020, 0.038, 0.040},
		master.Vec3{0.006, 0.014, 0.017},
	)
	c.rect(0, 276, width, height-276, master.Vec3{0.025, 0.023, 0.020})
	faceX := 320.0
	faceY := 178.0
	skinBase := master.Vec3{0.290, 0.155, 0.095}
	c.ellipse(faceX, faceY, 108, 139, func(x, y int, radial float64) master.Vec3 {
		nx := (float64(x) - faceX) / 108
		ny := (float64(y) - faceY) / 139
		light := 1.02 - 0.42*nx + 0.22*nx*nx - 0.12*ny
		light += 0.18 * smoothstep(0.25, 0.95, radial)
		return scale(skinBase, math.Max(0.52, light))
	})
	c.ellipse(faceX, faceY-122, 112, 34, func(_ int, _ int, _ float64) master.Vec3 {
		return master.Vec3{0.012, 0.008, 0.005}
	})
	for _, side := range []float64{-42, 42} {
		c.ellipse(faceX+side, faceY-26, 25, 12, func(_ int, _ int, t float64) master.Vec3 {
			return master.Vec3{0.72 * t, 0.60 * t, 0.54 * t}
		})
		c.circle(faceX+side, faceY-26, 7, func(t float64) master.Vec3 {
			return master.Vec3{0.014 + 0.01*t, 0.008 + 0.006*t, 0.005 + 0.004*t}
		})
	}
	c.ellipse(faceX, faceY+28, 13, 36, func(_ int, _ int, t float64) master.Vec3 {
		return scale(skinBase, 1.18-0.25*t)
	})
	c.ellipse(faceX, faceY+78, 43, 12, func(_ int, _ int, t float64) master.Vec3 {
		return master.Vec3{0.34 * t, 0.060 * t, 0.050 * t}
	})
	c.circle(faceX-86, faceY-58, 8, func(t float64) master.Vec3 {
		return master.Vec3{2.8 * t, 2.2 * t, 1.8 * t}
	})
	c.rect(grayX-5, grayY-5, graySize+10, graySize+10, master.Vec3{0.004, 0.004, 0.004})
	c.rect(grayX, grayY, graySize, graySize, gray18())
	skinROI := master.Rect{X: 282, Y: 150, Width: 58, Height: 54}
	return c.f, &master.FrameAnalysis{
		MiddleGray: &middleGray,
		Skin:       &skinROI,
	}
}

func timecode(frame int) string {
	seconds := (frame - 1) / 24
	return fmt.Sprintf("%02d:%02d:%02d:%02d", seconds/3600, (seconds/60)%60, seconds%60, (frame-1)%24)
}
