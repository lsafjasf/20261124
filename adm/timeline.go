package adm

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// TimelineRow is one (segment, object) row of the unified timeline.
type TimelineRow struct {
	Start   float64
	End     float64 // -1 = open
	Object  string
	Az      float64
	El      float64
	Energy  []float64 // per layout speaker, power
	Mapping []string  // predicted dominant channels
	Events  []string
}

// Timeline merges all block boundaries into segments and evaluates every
// object's position, per-channel energy and predicted mapping per segment.
func Timeline(res *RenderResult) []TimelineRow {
	bounds := map[float64]bool{}
	maxEnd := 0.0
	for _, pb := range res.Blocks {
		bounds[pb.Start] = true
		if pb.End > 0 {
			bounds[pb.End] = true
			if pb.End > maxEnd {
				maxEnd = pb.End
			}
		}
	}
	var ts []float64
	for t := range bounds {
		ts = append(ts, t)
	}
	sort.Float64s(ts)
	if len(ts) == 0 {
		return nil
	}
	if ts[len(ts)-1] < maxEnd {
		ts = append(ts, maxEnd)
	}
	var rows []TimelineRow
	for i := 0; i < len(ts); i++ {
		segStart := ts[i]
		segEnd := -1.0
		if i+1 < len(ts) {
			segEnd = ts[i+1]
		}
		for _, pb := range res.Blocks {
			if !activeIn(pb, segStart, segEnd) {
				continue
			}
			row := TimelineRow{
				Start:  segStart,
				End:    segEnd,
				Object: pb.Object,
				Az:     pb.Pos.Azimuth,
				El:     pb.Pos.Elevation,
				Energy: pb.Energy,
			}
			row.Mapping = predictMapping(pb.Energy, res.Layout)
			for _, ev := range pb.Events {
				row.Events = append(row.Events, ev.Stage+": "+ev.Detail)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func activeIn(pb *PannedBlock, start, end float64) bool {
	pbEnd := pb.End
	if pbEnd < 0 {
		pbEnd = math.Inf(1)
	}
	segEnd := end
	if segEnd < 0 {
		segEnd = math.Inf(1)
	}
	return pb.Start < segEnd && pbEnd > start
}

// predictMapping returns the channels carrying >= 25% of the block energy.
func predictMapping(energy []float64, layout Layout) []string {
	var total float64
	for _, e := range energy {
		total += e
	}
	if total == 0 {
		return []string{"（无输出）"}
	}
	type kv struct {
		name string
		frac float64
	}
	var parts []kv
	for i, s := range layout.Speakers {
		if frac := energy[i] / total; frac >= 0.25 {
			parts = append(parts, kv{s.Short, frac})
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].frac > parts[j].frac })
	if len(parts) == 0 {
		// Energy is too spread out: report the top channel.
		best, bi := 0.0, -1
		for i, s := range layout.Speakers {
			if !s.LFE && energy[i] > best {
				best, bi = energy[i], i
			}
		}
		if bi >= 0 {
			return []string{layout.Speakers[bi].Short + "（分散）"}
		}
		return []string{"（分散）"}
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = fmt.Sprintf("%s(%.0f%%)", p.name, p.frac*100)
	}
	return out
}

// FormatEnergy renders the per-channel energy vector as dB values.
func FormatEnergy(energy []float64, layout Layout) string {
	var parts []string
	for i, s := range layout.Speakers {
		db := math.Inf(-1)
		if energy[i] > 0 {
			db = 10 * math.Log10(energy[i])
		}
		if db < -60 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%.1f", s.Short, db))
	}
	if len(parts) == 0 {
		return "（静音）"
	}
	return strings.Join(parts, " ")
}
