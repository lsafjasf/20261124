package inspect

import "sort"

// Feature4 combines spatial edge rank with temporal motion.
//
// Static set pieces (bands, subtitle bars) are identical between neighboring
// frames, so identity comes from (1) the moving marker's temporal delta and
// (2) spatial gradients. Both are rank-based, which makes the fingerprint
// invariant to the SDR gamma vs HDR PQ amplitude remapping.
type Feature4 struct {
	Frame   int
	Spatial []int // top spatial-gradient indexes
	Motion  []int // top temporal-difference indexes
	MeanY   int
}

const (
	spatialK = 160
	motionK  = 80
)

type gradPoint struct {
	idx int
	mag int
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// FeatureOf4 builds the fingerprint for frame idx using the previous frame
// for temporal differences (nil for the first frame).
func FeatureOf4(cur, prev vxFrame, width, height int) Feature4 {
	spatial := make([]gradPoint, 0, width*height/8)
	motion := make([]gradPoint, 0, width*height/16)
	sumY := 0
	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			i := y*width + x
			sumY += int(cur.Y[i])
			gx := absInt(int(cur.Y[i]) - int(cur.Y[i-1]))
			gy := absInt(int(cur.Y[i]) - int(cur.Y[i-width]))
			mag := gx
			if gy > mag {
				mag = gy
			}
			if mag >= 2 {
				spatial = append(spatial, gradPoint{idx: i, mag: mag})
			}
			if prev.Y != nil {
				d := absInt(int(cur.Y[i]) - int(prev.Y[i]))
				if d >= 2 {
					motion = append(motion, gradPoint{idx: i, mag: d})
				}
			}
		}
	}
	top := func(points []gradPoint, k int) []int {
		sort.Slice(points, func(i, j int) bool {
			if points[i].mag != points[j].mag {
				return points[i].mag > points[j].mag
			}
			return points[i].idx < points[j].idx
		})
		if len(points) > k {
			points = points[:k]
		}
		out := make([]int, len(points))
		for i, p := range points {
			out[i] = p.idx
		}
		return out
	}
	return Feature4{
		Frame:   cur.Index,
		Spatial: top(spatial, spatialK),
		Motion:  top(motion, motionK),
		MeanY:   sumY / (width * height),
	}
}

// MotionDistance compares temporal-difference edge sets with one-pixel
// tolerance, normalized to 0..1.
func MotionDistance(a, b Feature4, width int) float64 {
	if len(a.Motion) == 0 && len(b.Motion) == 0 {
		return 0
	}
	set := make(map[int]bool, len(b.Motion))
	for _, idx := range b.Motion {
		set[idx] = true
	}
	match := 0
	for _, idx := range a.Motion {
		if edgeNearby(set, idx, width) {
			match++
		}
	}
	den := len(a.Motion)
	if len(b.Motion) > den {
		den = len(b.Motion)
	}
	if den == 0 {
		return 1
	}
	return 1 - float64(match)/float64(den)
}

// SpatialDistance compares spatial edge ranks with one-pixel tolerance.
func SpatialDistance(a, b Feature4, width int) float64 {
	set := make(map[int]bool, len(b.Spatial))
	for _, idx := range b.Spatial {
		set[idx] = true
	}
	match := 0
	for _, idx := range a.Spatial {
		if edgeNearby(set, idx, width) {
			match++
		}
	}
	return 1 - float64(match)/float64(len(a.Spatial))
}

// CombinedDistance weights motion identity above static geometry.
func CombinedDistance(a, b Feature4, width int) float64 {
	return 0.65*MotionDistance(a, b, width) + 0.35*SpatialDistance(a, b, width)
}

func edgeNearby(set map[int]bool, idx, width int) bool {
	for _, dy := range []int{-width, 0, width} {
		for _, dx := range []int{-1, 0, 1} {
			if set[idx+dy+dx] {
				return true
			}
		}
	}
	return false
}
