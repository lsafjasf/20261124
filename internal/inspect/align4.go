package inspect

import "vxcheck/internal/vx"

// BuildFeatures computes the rank-based fingerprints for every frame.
func BuildFeatures(f *vx.File, width, height int) []Feature4 {
	out := make([]Feature4, len(f.Frames))
	for i := range f.Frames {
		var prev vx.Frame
		if i > 0 {
			prev = f.Frames[i-1]
		}
		out[i] = FeatureOf4(f.Frames[i], prev, width, height)
	}
	return out
}

const (
	// matchLimit: same-content pairs observed under 0.43, different content
	// above 0.18 in the worst case; 0.55 accepts post-drop motion seams while
	// rejecting true scene mismatches.
	matchLimit4  = 0.55
	skipPenalty4 = 0.6
)

// Align4 performs monotonic dynamic-programming alignment using the rank
// fingerprint distance. It tolerates single-frame drops on either timeline.
func Align4(sdr, hdr []Feature4, width int) Alignment {
	n, m := len(sdr), len(hdr)
	type cell struct {
		cost float64
		op   byte
	}
	dp := make([][]cell, n+1)
	for i := range dp {
		dp[i] = make([]cell, m+1)
	}
	for i := 1; i <= n; i++ {
		dp[i][0] = cell{cost: float64(i) * skipPenalty4, op: 1}
	}
	for j := 1; j <= m; j++ {
		dp[0][j] = cell{cost: float64(j) * skipPenalty4, op: 2}
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			d := CombinedDistance(sdr[i-1], hdr[j-1], width)
			matchCost := dp[i-1][j-1].cost + d
			skipSDR := dp[i-1][j].cost + skipPenalty4
			skipHDR := dp[i][j-1].cost + skipPenalty4

			best, op := matchCost, byte(0)
			if skipSDR < best {
				best, op = skipSDR, 1
			}
			if skipHDR < best {
				best, op = skipHDR, 2
			}
			dp[i][j] = cell{cost: best, op: op}
		}
	}

	var al Alignment
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && dp[i][j].op == 0:
			d := CombinedDistance(sdr[i-1], hdr[j-1], width)
			al.Pairs = append(al.Pairs, Pair{
				SDRIndex:  i - 1,
				HDRIndex:  j - 1,
				EdgeDiff:  d,
				MeanLumaD: sdr[i-1].MeanY - hdr[j-1].MeanY,
			})
			i--
			j--
		case i > 0 && (j == 0 || dp[i][j].op == 1):
			al.SDROnly = append(al.SDROnly, i-1)
			i--
		default:
			al.HDROnly = append(al.HDROnly, j-1)
			j--
		}
	}
	reversePairs(al.Pairs)
	reverseInts(al.SDROnly)
	reverseInts(al.HDROnly)

	kept := al.Pairs[:0]
	for _, p := range al.Pairs {
		if p.EdgeDiff <= matchLimit4 {
			kept = append(kept, p)
		} else {
			al.SDROnly = append(al.SDROnly, p.SDRIndex)
			al.HDROnly = append(al.HDROnly, p.HDRIndex)
		}
	}
	al.Pairs = kept
	return al
}
