package pgs

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"
)

type decoder struct {
	palettes    map[uint8]*parsedPalette
	objects     map[uint16]*parsedObject
	partial     map[uint16]*parsedObject
	windows     map[uint8]parsedWindow
	composition *parsedComposition
	videoHeight int
}

func newDecoder() *decoder {
	return &decoder{
		palettes: make(map[uint8]*parsedPalette),
		objects:  make(map[uint16]*parsedObject),
		partial:  make(map[uint16]*parsedObject),
		windows:  make(map[uint8]parsedWindow),
	}
}

func (d *decoder) reset() {
	d.palettes = make(map[uint8]*parsedPalette)
	d.objects = make(map[uint16]*parsedObject)
	d.partial = make(map[uint16]*parsedObject)
	d.windows = make(map[uint8]parsedWindow)
	d.composition = nil
}

func (s *Stream) stateAt(at time.Duration) (*decoder, error) {
	ticks, err := durationTicks(at)
	if err != nil {
		return nil, err
	}
	return s.stateAtTicks(ticks)
}

func (s *Stream) stateAtTicks(ticks uint32) (*decoder, error) {
	sets := append([]displaySet(nil), s.sets...)
	sort.SliceStable(sets, func(i, j int) bool { return sets[i].pts < sets[j].pts })

	d := newDecoder()
	for _, set := range sets {
		if set.pts > ticks {
			continue
		}
		if err := d.applyDisplaySet(set); err != nil {
			return nil, fmt.Errorf("pgs: display set at %s: %w", ticksDuration(set.pts), err)
		}
	}
	if d.composition == nil {
		return nil, ErrNoComposition
	}
	return d, nil
}

func (d *decoder) applyDisplaySet(set displaySet) error {
	var pcs *parsedComposition
	for i := range set.segments {
		if set.segments[i].Type != SegmentPresentation {
			continue
		}
		parsed, err := parsePresentation(set.segments[i].Payload)
		if err != nil {
			return err
		}
		pcs = parsed
		break
	}
	if pcs == nil {
		return nil
	}

	if pcs.state != 0 {
		d.reset()
	}
	d.videoHeight = pcs.height

	for i := range set.segments {
		seg := &set.segments[i]
		var err error
		switch seg.Type {
		case SegmentPalette:
			err = d.applyPalette(seg.Payload)
		case SegmentObject:
			err = d.applyObject(seg.Payload)
		case SegmentWindow:
			err = d.applyWindows(seg.Payload)
		}
		if err != nil {
			return fmt.Errorf("segment type 0x%02x at file offset %d: %w", seg.Type, seg.Offset, err)
		}
	}

	if pcs.paletteUpdate && len(pcs.objects) == 0 && d.composition != nil {
		d.composition.pts = set.pts
		d.composition.width = pcs.width
		d.composition.height = pcs.height
		d.composition.number = pcs.number
		d.composition.state = pcs.state
		d.composition.paletteUpdate = true
		d.composition.paletteID = pcs.paletteID
		return nil
	}

	pcs.pts = set.pts
	d.composition = pcs
	return nil
}

func parsePresentation(p []byte) (*parsedComposition, error) {
	if len(p) < 11 {
		return nil, fmt.Errorf("presentation segment is %d bytes, want at least 11", len(p))
	}
	pcs := &parsedComposition{
		width:         int(binary.BigEndian.Uint16(p[0:2])),
		height:        int(binary.BigEndian.Uint16(p[2:4])),
		number:        binary.BigEndian.Uint16(p[5:7]),
		state:         p[7] >> 6,
		paletteUpdate: p[8]&0x80 != 0,
		paletteID:     p[9],
	}
	if pcs.width <= 0 || pcs.height <= 0 {
		return nil, fmt.Errorf("invalid video size %dx%d", pcs.width, pcs.height)
	}

	count := int(p[10])
	pos := 11
	pcs.objects = make([]compositionObject, 0, count)
	for i := 0; i < count; i++ {
		need := 8
		if pos+need > len(p) {
			return nil, fmt.Errorf("presentation object %d truncated at byte %d", i, pos)
		}
		obj := compositionObject{
			id:       binary.BigEndian.Uint16(p[pos : pos+2]),
			windowID: p[pos+2],
			cropped:  p[pos+3]&0x80 != 0,
			forced:   p[pos+3]&0x40 != 0,
			x:        int(binary.BigEndian.Uint16(p[pos+4 : pos+6])),
			y:        int(binary.BigEndian.Uint16(p[pos+6 : pos+8])),
		}
		pos += 8
		if obj.cropped {
			if pos+8 > len(p) {
				return nil, fmt.Errorf("presentation object %d crop is truncated", i)
			}
			obj.crop = Rect{
				X:      int(binary.BigEndian.Uint16(p[pos : pos+2])),
				Y:      int(binary.BigEndian.Uint16(p[pos+2 : pos+4])),
				Width:  int(binary.BigEndian.Uint16(p[pos+4 : pos+6])),
				Height: int(binary.BigEndian.Uint16(p[pos+6 : pos+8])),
			}
			pos += 8
		}
		pcs.objects = append(pcs.objects, obj)
	}
	if pos != len(p) {
		return nil, fmt.Errorf("presentation segment has %d trailing bytes", len(p)-pos)
	}
	return pcs, nil
}

func (d *decoder) applyPalette(p []byte) error {
	if len(p) < 2 {
		return fmt.Errorf("palette segment is %d bytes, want at least 2", len(p))
	}
	if (len(p)-2)%5 != 0 {
		return fmt.Errorf("palette segment has incomplete entry (%d bytes)", len(p))
	}

	id, version := p[0], p[1]
	palette := d.palettes[id]
	if palette == nil {
		palette = &parsedPalette{id: id}
		d.palettes[id] = palette
	}
	palette.version = version

	for pos := 2; pos < len(p); pos += 5 {
		index := p[pos]
		palette.entries[index] = ycbcrToRGBA(p[pos+1], p[pos+2], p[pos+3], p[pos+4], d.videoHeight)
		palette.present[index] = true
	}
	return nil
}

func (d *decoder) applyWindows(p []byte) error {
	if len(p) == 0 {
		return fmt.Errorf("empty window segment")
	}
	count := int(p[0])
	if len(p) != 1+count*9 {
		return fmt.Errorf("window segment is %d bytes, want %d", len(p), 1+count*9)
	}
	pos := 1
	for i := 0; i < count; i++ {
		w := parsedWindow{
			id: p[pos],
			rect: Rect{
				X:      int(binary.BigEndian.Uint16(p[pos+1 : pos+3])),
				Y:      int(binary.BigEndian.Uint16(p[pos+3 : pos+5])),
				Width:  int(binary.BigEndian.Uint16(p[pos+5 : pos+7])),
				Height: int(binary.BigEndian.Uint16(p[pos+7 : pos+9])),
			},
		}
		d.windows[w.id] = w
		pos += 9
	}
	return nil
}

func (d *decoder) applyObject(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("object segment is %d bytes, want at least 4", len(p))
	}
	id := binary.BigEndian.Uint16(p[0:2])
	version := p[2]
	flags := p[3]

	obj := d.partial[id]
	if flags&0x80 != 0 {
		if len(p) < 7 {
			return fmt.Errorf("object %d first segment is %d bytes, want at least 7", id, len(p))
		}
		declared := int(p[4])<<16 | int(p[5])<<8 | int(p[6])
		data := p[7:]
		if len(data) < 4 {
			return fmt.Errorf("object %d first segment lacks dimensions", id)
		}
		if declared < 4 {
			return fmt.Errorf("object %d declares invalid data length %d", id, declared)
		}
		obj = &parsedObject{
			id:      id,
			version: version,
			width:   int(binary.BigEndian.Uint16(data[0:2])),
			height:  int(binary.BigEndian.Uint16(data[2:4])),
		}
		obj.data = append(obj.data, data[4:]...)
		d.partial[id] = obj
	} else {
		if obj == nil {
			return fmt.Errorf("object %d continuation has no first segment", id)
		}
		if len(p) < 7 {
			return fmt.Errorf("object %d continuation is %d bytes, want at least 7", id, len(p))
		}
		obj.data = append(obj.data, p[7:]...)
	}

	if flags&0x40 == 0 {
		return nil
	}
	pixels, err := decodeRLE(obj.data, obj.width, obj.height)
	if err != nil {
		return fmt.Errorf("object %d: %w", id, err)
	}
	obj.pixels = pixels
	obj.complete = true
	obj.data = nil
	d.objects[id] = obj
	delete(d.partial, id)
	return nil
}

func durationTicks(d time.Duration) (uint32, error) {
	if d < 0 {
		return 0, fmt.Errorf("pgs: negative timestamp %s", d)
	}
	ticks := d/time.Second*Clock + (d%time.Second)*Clock/time.Second
	if ticks > math.MaxUint32 {
		return 0, fmt.Errorf("pgs: timestamp %s exceeds 32-bit PTS", d)
	}
	return uint32(ticks), nil
}

func ticksDuration(ticks uint32) time.Duration {
	return time.Duration(int64(ticks) * int64(time.Second) / Clock)
}
