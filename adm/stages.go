package adm

import (
	"fmt"
	"math"
)

// normalizePosition implements stage S2: coordinate normalisation.
func normalizePosition(pb *PannedBlock, cf *ChannelFormat, blk Block, res *RenderResult) Position {
	ev := func(d string) {
		pb.Events = append(pb.Events, FoldEvent{Stage: StageNormalized, Object: pb.Object, Detail: d})
	}
	if !blk.HasPosition {
		ev("块缺少 position 元素，按默认 (0°, 0°) 处理——对象会落在中置")
		return Position{Azimuth: 0, Elevation: 0, Distance: 1}
	}
	p := blk.Pos
	if p.Cartesian && !cf.Cartesian {
		ev("坐标系不匹配：块是 cartesian 而 channelFormat 是 polar，X/Y/Z 被丢弃为 (0°, 0°)")
		return Position{Azimuth: 0, Elevation: 0, Distance: 1}
	}
	if !p.Cartesian && cf.Cartesian {
		ev("坐标系不匹配：channelFormat 声明 cartesian 但块给的是 az/el，按 (0°, 0°) 处理")
		return Position{Azimuth: 0, Elevation: 0, Distance: 1}
	}
	if p.Cartesian {
		p = cartesianToPolar(p)
	}
	if blk.ScreenEdgeLock && math.Abs(p.Azimuth) > 30 {
		ev(fmt.Sprintf("screenEdgeLock 将方位角 %.1f° 钳制到 ±30°", p.Azimuth))
		p.Azimuth = math.Copysign(30, p.Azimuth)
	}
	return p
}

func cartesianToPolar(p Position) Position {
	az := math.Atan2(-p.X, p.Y) * 180 / math.Pi
	el := math.Atan2(p.Z, math.Hypot(p.X, p.Y)) * 180 / math.Pi
	d := math.Sqrt(p.X*p.X + p.Y*p.Y + p.Z*p.Z)
	return Position{Azimuth: az, Elevation: el, Distance: d}
}

// panBlock implements stage S3: panning onto the target layout.
func panBlock(pb *PannedBlock, cf *ChannelFormat, blk Block, layout Layout, res *RenderResult) []float64 {
	ev := func(d string) {
		pb.Events = append(pb.Events, FoldEvent{Stage: StagePanned, Object: pb.Object, Detail: d})
	}
	if cf.Type == "DirectSpeakers" {
		return panDirectSpeakers(pb, cf, layout, ev)
	}
	energy := make([]float64, len(layout.Speakers))
	maxEl := 0.0
	for _, s := range layout.Speakers {
		if s.Elevation > maxEl {
			maxEl = s.Elevation
		}
	}
	el := pb.Pos.Elevation
	if maxEl == 0 && math.Abs(el) > 20 {
		ev(fmt.Sprintf("目标布局无高度声道，高度 %.1f° 被丢弃，能量压回屏幕平面", el))
		el = 0
	}
	src := unitVec(pb.Pos.Azimuth, el)
	var weights []float64
	var sumSq float64
	for _, s := range layout.Speakers {
		if s.LFE {
			weights = append(weights, 0)
			continue
		}
		w := math.Max(0, dot(src, unitVec(s.Azimuth, s.Elevation)))
		weights = append(weights, w)
		sumSq += w * w
	}
	if sumSq == 0 {
		ev("声像方向落在所有扬声器背面，全部能量回退到中置")
		return allToCenter(pb, layout)
	}
	// Extent: blend part of the energy uniformly over the non-LFE speakers.
	spread := math.Min(1, math.Max(blk.Width, blk.Height)) * 0.5
	// Divergence: route part of the energy to the centre.
	div := math.Max(0, math.Min(1, blk.Divergence.Value))
	norm := math.Sqrt(sumSq)
	nSpk := 0
	for _, s := range layout.Speakers {
		if !s.LFE {
			nSpk++
		}
	}
	for i, s := range layout.Speakers {
		if s.LFE {
			continue
		}
		g := weights[i] / norm
		p := g * g * (1 - spread) * (1 - div)
		p += spread * (1 - div) / float64(nSpk)
		if s.Short == "C" {
			p += div
		}
		energy[i] = p * pb.BasePower
	}
	if div > 0 {
		ev(fmt.Sprintf("divergence=%.2f 将 %.0f%% 能量推向中置", div, div*100))
	}
	return energy
}

func allToCenter(pb *PannedBlock, layout Layout) []float64 {
	energy := make([]float64, len(layout.Speakers))
	for i, s := range layout.Speakers {
		if s.Short == "C" {
			energy[i] = pb.BasePower
			return energy
		}
	}
	// No dedicated centre: split L/R.
	for i, s := range layout.Speakers {
		if s.Short == "L" || s.Short == "R" {
			energy[i] = pb.BasePower / 2
		}
	}
	return energy
}
