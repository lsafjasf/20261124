package adm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParseADMDuration parses ADM time values ("HH:MM:SS.fffff" or "SS.fffff")
// into a Duration. Empty string yields 0 with ok=false.
func ParseADMDuration(s string) (time.Duration, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, nil
	}
	parts := strings.Split(s, ":")
	var sec string
	var h, m int
	switch len(parts) {
	case 1:
		sec = parts[0]
	case 3:
		var err error
		if h, err = strconv.Atoi(parts[0]); err != nil {
			return 0, false, fmt.Errorf("bad hours in %q", s)
		}
		if m, err = strconv.Atoi(parts[1]); err != nil {
			return 0, false, fmt.Errorf("bad minutes in %q", s)
		}
		sec = parts[2]
	default:
		return 0, false, fmt.Errorf("unsupported time format %q", s)
	}
	f, err := strconv.ParseFloat(sec, 64)
	if err != nil {
		return 0, false, fmt.Errorf("bad seconds in %q", s)
	}
	d := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
		time.Duration(f*float64(time.Second))
	return d, true, nil
}

// Block is one timeline segment of a channel format.
type Block struct {
	ID        string
	Start     time.Duration
	End       time.Duration
	HasTime   bool
	Positions map[string]float64
}

// ChannelTimeline groups the blocks of one channel format.
type ChannelTimeline struct {
	ChannelID   string
	ChannelName string
	TypeDef     string
	Blocks      []Block
}

// ObjectTimeline is the probe result for one audioObject.
type ObjectTimeline struct {
	ObjectID   string
	ObjectName string
	Start      time.Duration
	End        time.Duration
	HasTime    bool
	Tracks     []uint16 // physical tracks resolved via chna, empty if unattached
	Channels   []ChannelTimeline
}

// ProbeTimeline builds the object timeline view. It is an analysis of
// declared metadata only: nothing here implies the objects were rendered.
func ProbeTimeline(f *File) ([]ObjectTimeline, []Finding) {
	var out []ObjectTimeline
	var findings []Finding
	chnaTrack := map[string]uint16{}
	if f.Chna != nil {
		for _, e := range f.Chna.Entries {
			chnaTrack[e.UID] = e.TrackIndex
		}
	}
	packs := map[string]PackFormat{}
	for _, p := range f.Doc.Packs {
		packs[p.ID] = p
	}
	channels := map[string]ChannelFormat{}
	for _, c := range f.Doc.Channels {
		channels[c.ID] = c
	}
	for _, o := range f.Doc.Objects {
		ot := ObjectTimeline{ObjectID: o.ID, ObjectName: o.Name}
		start, okS, err := ParseADMDuration(o.Start)
		if err != nil {
			findings = append(findings, Finding{Warn, "OBJ-TIME", fmt.Sprintf("audioObject %s: %v", o.ID, err)})
		}
		dur, okD, err := ParseADMDuration(o.Duration)
		if err != nil {
			findings = append(findings, Finding{Warn, "OBJ-TIME", fmt.Sprintf("audioObject %s: %v", o.ID, err)})
		}
		if okS && okD {
			ot.Start, ot.End, ot.HasTime = start, start+dur, true
		}
		for _, ref := range o.TrackUIDRefs {
			if trk, ok := chnaTrack[ref]; ok {
				ot.Tracks = append(ot.Tracks, trk)
			}
		}
		sort.Slice(ot.Tracks, func(i, j int) bool { return ot.Tracks[i] < ot.Tracks[j] })
		for _, packID := range o.PackRefIDs {
			p, ok := packs[packID]
			if !ok {
				continue // already reported as MISSING-REF
			}
			for _, chID := range collectChannels(p, packs) {
				cf, ok := channels[chID]
				if !ok {
					continue
				}
				ct := ChannelTimeline{ChannelID: cf.ID, ChannelName: cf.Name, TypeDef: cf.TypeDef}
				for _, bf := range cf.BlockFormats {
					blk := Block{ID: bf.ID, Positions: map[string]float64{}}
					rt, hasR, err := ParseADMDuration(bf.RTime)
					if err != nil {
						findings = append(findings, Finding{Warn, "BLK-TIME", fmt.Sprintf("audioBlockFormat %s: %v", bf.ID, err)})
					}
					bd, hasD, err := ParseADMDuration(bf.Duration)
					if err != nil {
						findings = append(findings, Finding{Warn, "BLK-TIME", fmt.Sprintf("audioBlockFormat %s: %v", bf.ID, err)})
					}
					if hasR && hasD {
						blk.Start, blk.End, blk.HasTime = rt, rt+bd, true
					}
					for _, pos := range bf.Positions {
						blk.Positions[pos.Coordinate] = pos.Value
					}
					ct.Blocks = append(ct.Blocks, blk)
				}
				ot.Channels = append(ot.Channels, ct)
			}
		}
		out = append(out, ot)
	}
	for i := range out {
		for _, ct := range out[i].Channels {
			findings = append(findings, checkBlockContinuity(out[i].ObjectID, ct)...)
		}
	}
	return out, findings
}

// collectChannels flattens a pack (including nested packs) into channel IDs.
func collectChannels(p PackFormat, packs map[string]PackFormat) []string {
	var ids []string
	ids = append(ids, p.ChannelRefIDs...)
	for _, sub := range p.PackRefIDs {
		if sp, ok := packs[sub]; ok {
			ids = append(ids, collectChannels(sp, packs)...)
		}
	}
	return ids
}

// checkBlockContinuity flags gaps and overlaps between consecutive
// audioBlockFormats of one channel format.
func checkBlockContinuity(objID string, ct ChannelTimeline) []Finding {
	var out []Finding
	blocks := ct.Blocks
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Start < blocks[j].Start })
	for i := 1; i < len(blocks); i++ {
		prev, cur := blocks[i-1], blocks[i]
		if !prev.HasTime || !cur.HasTime {
			continue
		}
		switch {
		case cur.Start > prev.End:
			out = append(out, Finding{Warn, "BLK-GAP",
				fmt.Sprintf("audioObject %s / %s: gap of %s between block %s (ends %s) and block %s (starts %s)",
					objID, ct.ChannelID, cur.Start-prev.End, prev.ID, prev.End, cur.ID, cur.Start)})
		case cur.Start < prev.End:
			out = append(out, Finding{Warn, "BLK-OVERLAP",
				fmt.Sprintf("audioObject %s / %s: block %s starts %s before block %s ends",
					objID, ct.ChannelID, cur.ID, prev.End-cur.Start, prev.ID)})
		}
	}
	return out
}
