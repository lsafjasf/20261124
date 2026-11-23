package adm

import (
	"fmt"
	"sort"
)

// Stage IDs in the simulated processing chain.
const (
	StageSource     = "S1" // authored position
	StageNormalized = "S2" // coordinate normalisation / ingest
	StagePanned     = "S3" // panning onto the target layout
	StageDownmix    = "S4" // optional LoRo downmix
)

// FoldEvent records one object being folded/dropped at one stage.
type FoldEvent struct {
	Stage  string
	Object string
	Detail string
}

// PannedBlock is one block of one object carried through the chain.
type PannedBlock struct {
	Object    string
	Channel   string
	Start     float64
	End       float64 // -1 = open ended
	Authored  Position
	Pos       Position  // position after S2
	Energy    []float64 // per layout-speaker power after final stage
	BasePower float64   // object*block gain squared
	Events    []FoldEvent
}

// RenderResult is the full simulated chain output.
type RenderResult struct {
	Layout   Layout
	Downmix  bool
	Blocks   []*PannedBlock
	Events   []FoldEvent
	Warnings []string
}

// Render resolves objects to channel blocks and pushes every block through
// the four processing stages against the target layout.
func Render(doc *Document, layout Layout, downmix bool) *RenderResult {
	res := &RenderResult{Layout: layout, Downmix: downmix, Warnings: doc.Warnings}
	for _, obj := range doc.Objects {
		channels := resolveChannels(doc, obj)
		if len(channels) == 0 {
			res.Events = append(res.Events, FoldEvent{Stage: StageSource, Object: obj.Name,
				Detail: "对象没有解析到任何 audioChannelFormat（检查 audioPackFormatIDRef 链）"})
			continue
		}
		for _, cf := range channels {
			if len(cf.Blocks) == 0 {
				res.Events = append(res.Events, FoldEvent{Stage: StageSource, Object: obj.Name,
					Detail: fmt.Sprintf("声道格式 %s 没有任何 audioBlockFormat", cf.ID)})
				continue
			}
			for i, blk := range cf.Blocks {
				pb := &PannedBlock{
					Object:   obj.Name,
					Channel:  cf.Name,
					Start:    blk.Rtime,
					End:      blockEnd(cf, i),
					Authored: blk.Pos,
				}
				pb.BasePower = obj.Gain * obj.Gain * blk.Gain * blk.Gain
				pb.Pos = normalizePosition(pb, cf, blk, res)
				pb.Energy = panBlock(pb, cf, blk, layout, res)
				if downmix {
					pb.Energy = downmixLoRo(pb, layout, res)
				}
				res.Blocks = append(res.Blocks, pb)
			}
		}
	}
	sort.Slice(res.Blocks, func(i, j int) bool {
		if res.Blocks[i].Start != res.Blocks[j].Start {
			return res.Blocks[i].Start < res.Blocks[j].Start
		}
		return res.Blocks[i].Object < res.Blocks[j].Object
	})
	return res
}

func resolveChannels(doc *Document, obj Object) []*ChannelFormat {
	var out []*ChannelFormat
	for _, pref := range obj.PackRefs {
		pf, ok := doc.PackFormats[pref]
		if !ok {
			continue
		}
		for _, cref := range pf.ChannelRefs {
			if cf, ok := doc.ChannelFormats[cref]; ok {
				out = append(out, cf)
			}
		}
	}
	return out
}

func blockEnd(cf *ChannelFormat, i int) float64 {
	blk := cf.Blocks[i]
	if blk.Duration > 0 {
		return blk.Rtime + blk.Duration
	}
	if i+1 < len(cf.Blocks) {
		return cf.Blocks[i+1].Rtime
	}
	return -1
}
