package pgs

import "fmt"

// DisplaySet is the group of segments between two clear (END) segments.
type DisplaySet struct {
	Index        int
	StartPTS     uint32 // PTS of the first segment of the set
	EndPTS       uint32 // PTS of the closing clear segment
	SegmentFirst int
	SegmentLast  int
}

// Epoch is the rebuilt composition state of one epoch. Palettes, windows
// and objects only ever contain definitions that belong to this epoch:
// nothing is carried over from a previous epoch.
type Epoch struct {
	Index    int
	StartPTS uint32
	EndPTS   uint32

	Palettes map[byte]*PaletteSegment
	Windows  map[byte]WindowDef
	Objects  map[uint16]*ObjectSegment

	Sets []int // display set indexes belonging to this epoch
}

// RebuildError pinpoints a segment that broke epoch rules.
type RebuildError struct {
	SegmentIndex int
	SegmentType  byte
	PTS          uint32
	Message      string
}

func (e RebuildError) String() string {
	return fmt.Sprintf("segment %d (%s) @ %s: %s",
		e.SegmentIndex, TypeName(e.SegmentType), FormatPTS(e.PTS), e.Message)
}

// Report is the result of rebuilding epoch state from a segment stream.
type Report struct {
	Segments   []Segment
	Sets       []DisplaySet
	Epochs     []Epoch
	Errors     []RebuildError
	FirstError *RebuildError
}

// Rebuild walks segments in arrival order (palette, window, object,
// display, clear as they appear on the wire) and reconstructs epoch
// state. Definitions are buffered per display set: when the set's
// display segment starts a new epoch, the buffered definitions belong
// to the new epoch; otherwise they merge into the current one. State
// is never carried across an epoch boundary, so a palette from an old
// epoch cannot be reused. A composition left on screen without a clear
// segment is reported as a residual window.
func Rebuild(segs []Segment) *Report {
	r := &Report{Segments: segs}
	b := &rebuilder{report: r, setStart: -1}
	b.resetSetBuffers()
	for i := range segs {
		b.step(&segs[i])
	}
	b.finish()
	if len(r.Errors) > 0 {
		r.FirstError = &r.Errors[0]
	}
	return r
}

type rebuilder struct {
	report    *Report
	epoch     *Epoch
	setStart  int  // segment index opening the current display set
	displayed bool // a composition is on screen, waiting for clear

	// Definitions arrived so far in the current display set. Their
	// owning epoch is decided when the set's display segment arrives.
	setPalettes map[byte]*PaletteSegment
	setWindows  map[byte]WindowDef
	setObjects  map[uint16]*ObjectSegment
}

func (b *rebuilder) resetSetBuffers() {
	b.setPalettes = map[byte]*PaletteSegment{}
	b.setWindows = map[byte]WindowDef{}
	b.setObjects = map[uint16]*ObjectSegment{}
}

func (b *rebuilder) fail(seg *Segment, format string, args ...any) {
	b.report.Errors = append(b.report.Errors, RebuildError{
		SegmentIndex: seg.Index,
		SegmentType:  seg.Type,
		PTS:          seg.PTS,
		Message:      fmt.Sprintf(format, args...),
	})
}

func (b *rebuilder) step(seg *Segment) {
	if b.setStart < 0 {
		b.setStart = seg.Index
	}
	switch seg.Type {
	case TypePalette:
		b.setPalettes[seg.Palette.ID] = seg.Palette
	case TypeWindow:
		for _, w := range seg.Window.Windows {
			b.setWindows[w.ID] = w
		}
	case TypeObject:
		if seg.Object.Sequence&SeqFirst == 0 {
			if _, ok := b.setObjects[seg.Object.ID]; !ok {
				b.fail(seg, "object %d continuation without first fragment", seg.Object.ID)
			}
			return
		}
		b.setObjects[seg.Object.ID] = seg.Object
	case TypeDisplay:
		b.onDisplay(seg)
	case TypeClear:
		b.onClear(seg)
	}
}

// mergeSetBuffers folds the current display set's definitions into the
// open epoch (used for mid-epoch display sets).
func (b *rebuilder) mergeSetBuffers() {
	if b.epoch == nil {
		return
	}
	for id, p := range b.setPalettes {
		b.epoch.Palettes[id] = p
	}
	for id, w := range b.setWindows {
		b.epoch.Windows[id] = w
	}
	for id, o := range b.setObjects {
		b.epoch.Objects[id] = o
	}
}

func (b *rebuilder) onDisplay(seg *Segment) {
	d := seg.Display
	if d.CompositionState == StateEpochStart {
		if b.displayed {
			b.fail(seg, "residual window: previous composition never cleared before epoch start")
			// Close the display set left open by the missing clear,
			// attributing it to the old epoch.
			if b.setStart >= 0 && b.setStart < seg.Index {
				b.report.Sets = append(b.report.Sets, DisplaySet{
					Index:        len(b.report.Sets),
					StartPTS:     b.report.Segments[b.setStart].PTS,
					EndPTS:       seg.PTS,
					SegmentFirst: b.setStart,
					SegmentLast:  seg.Index - 1,
				})
				if b.epoch != nil {
					b.epoch.Sets = append(b.epoch.Sets, len(b.report.Sets)-1)
				}
				b.setStart = seg.Index
			}
		}
		b.closeEpoch(seg.PTS)
		// The new epoch adopts only the definitions of its own
		// display set; everything older is dropped.
		b.epoch = &Epoch{
			Index:    len(b.report.Epochs),
			StartPTS: seg.PTS,
			Palettes: b.setPalettes,
			Windows:  b.setWindows,
			Objects:  b.setObjects,
		}
		b.displayed = false
	} else if b.epoch == nil {
		b.fail(seg, "display segment arrived before any epoch start")
		return
	} else {
		b.mergeSetBuffers()
	}
	if len(d.Objects) > 0 {
		if _, ok := b.epoch.Palettes[d.PaletteID]; !ok {
			b.fail(seg, "palette %d not defined in epoch %d: cross-epoch palette reuse",
				d.PaletteID, b.epoch.Index)
		}
		for _, co := range d.Objects {
			if _, ok := b.epoch.Windows[co.WindowID]; !ok {
				b.fail(seg, "window %d not defined in epoch %d", co.WindowID, b.epoch.Index)
			}
			if _, ok := b.epoch.Objects[co.ObjectID]; !ok {
				b.fail(seg, "object %d not defined in epoch %d", co.ObjectID, b.epoch.Index)
			}
		}
	}
	b.displayed = len(d.Objects) > 0
	// The display segment consumed this set's definitions; anything
	// arriving after it belongs to the remainder of the set and must
	// not leak into a following epoch.
	b.resetSetBuffers()
}

func (b *rebuilder) onClear(seg *Segment) {
	if b.setStart < 0 {
		b.fail(seg, "clear segment without an open display set")
		return
	}
	b.mergeSetBuffers() // a set may carry definitions but no display
	set := DisplaySet{
		Index:        len(b.report.Sets),
		StartPTS:     b.report.Segments[b.setStart].PTS,
		EndPTS:       seg.PTS,
		SegmentFirst: b.setStart,
		SegmentLast:  seg.Index,
	}
	b.report.Sets = append(b.report.Sets, set)
	if b.epoch != nil {
		b.epoch.Sets = append(b.epoch.Sets, set.Index)
	}
	b.setStart = -1
	b.displayed = false
	b.resetSetBuffers()
}

func (b *rebuilder) closeEpoch(endPTS uint32) {
	if b.epoch == nil {
		return
	}
	b.epoch.EndPTS = endPTS
	b.report.Epochs = append(b.report.Epochs, *b.epoch)
	b.epoch = nil
}

func (b *rebuilder) finish() {
	if b.setStart >= 0 && len(b.report.Segments) > 0 {
		b.mergeSetBuffers()
		last := b.report.Segments[len(b.report.Segments)-1]
		b.report.Sets = append(b.report.Sets, DisplaySet{
			Index:        len(b.report.Sets),
			StartPTS:     b.report.Segments[b.setStart].PTS,
			EndPTS:       last.PTS,
			SegmentFirst: b.setStart,
			SegmentLast:  last.Index,
		})
		if b.epoch != nil {
			b.epoch.Sets = append(b.epoch.Sets, len(b.report.Sets)-1)
		}
	}
	if b.epoch != nil {
		end := b.epoch.StartPTS
		if len(b.epoch.Sets) > 0 {
			end = b.report.Sets[b.epoch.Sets[len(b.epoch.Sets)-1]].EndPTS
		}
		b.closeEpoch(end)
	}
}
