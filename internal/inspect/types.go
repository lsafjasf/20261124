package inspect

import "vxcheck/internal/vx"

// Pair links one SDR frame with one HDR frame after structural alignment.
type Pair struct {
	SDRIndex  int
	HDRIndex  int
	EdgeDiff  float64
	MeanLumaD int
}

// Alignment is the full monotonic structural match between two streams.
type Alignment struct {
	Pairs   []Pair
	SDROnly []int // SDR frame indexes with no HDR counterpart (drops)
	HDROnly []int // HDR frame indexes with no SDR counterpart
}

func reversePairs(p []Pair) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}

func reverseInts(p []int) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}

type vxFrame = vx.Frame
