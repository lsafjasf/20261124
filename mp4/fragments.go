package mp4

import (
	"encoding/binary"
	"fmt"

	"hdr10audit/hdr"
)

type tfhdInfo struct {
	trackID    uint32
	baseOffset int64
	hasBase    bool
	desc       int
	sampleSize int64
}

type trunInfo struct {
	dataOffset int64
	hasOffset  bool
	sizes      []int64
}

func (f *File) appendFragments(info *Info, top []Box) error {
	byID := make(map[uint32]int, len(info.Tracks))
	for i := range info.Tracks {
		byID[info.Tracks[i].ID] = i
	}
	for _, moof := range findAll(top, "moof") {
		kids, err := f.Children(moof)
		if err != nil {
			return fmt.Errorf("parse moof at %d: %w", moof.Start, err)
		}
		for _, traf := range findAll(kids, "traf") {
			trakKids, err := f.Children(traf)
			if err != nil {
				return fmt.Errorf("parse traf at %d: %w", traf.Start, err)
			}
			th, err := f.parseTFHD(trakKids)
			if err != nil {
				return err
			}
			ti, ok := byID[th.trackID]
			if !ok {
				info.Issues = append(info.Issues, hdr.Issue{
					Code: "fragment_unknown_track", Severity: hdr.SeverityWarning,
					Location: fmt.Sprintf("moof@%d", moof.Start),
					Message:  fmt.Sprintf("traf 引用未知轨道 %d", th.trackID),
				})
				continue
			}
			base := moof.Start
			if th.hasBase {
				base = th.baseOffset
			}
			cursor := base
			for _, trunBox := range findAll(trakKids, "trun") {
				tr, err := f.parseTRUN(trunBox, th.sampleSize)
				if err != nil {
					return err
				}
				if tr.hasOffset {
					cursor = base + tr.dataOffset
				}
				for _, size := range tr.sizes {
					info.Tracks[ti].Samples = append(info.Tracks[ti].Samples, Sample{
						Offset: cursor, Size: size, Description: th.desc,
					})
					cursor += size
				}
			}
		}
	}
	return nil
}

func (f *File) parseTFHD(children []Box) (tfhdInfo, error) {
	box := find(children, "tfhd")
	if box == nil {
		return tfhdInfo{}, fmt.Errorf("tfhd not found")
	}
	raw, err := f.Payload(*box, 0)
	if err != nil {
		return tfhdInfo{}, err
	}
	if len(raw) < 8 {
		return tfhdInfo{}, fmt.Errorf("tfhd too short")
	}
	flags := binary.BigEndian.Uint32(raw[0:4]) & 0x00ffffff
	out := tfhdInfo{trackID: binary.BigEndian.Uint32(raw[4:8]), desc: 1}
	off := 8
	need := func(n int) error {
		if len(raw)-off < n {
			return fmt.Errorf("tfhd optional fields truncated")
		}
		return nil
	}
	if flags&0x000001 != 0 {
		if err := need(8); err != nil {
			return out, err
		}
		out.baseOffset = int64(binary.BigEndian.Uint64(raw[off : off+8]))
		out.hasBase = true
		off += 8
	}
	if flags&0x000002 != 0 {
		if err := need(4); err != nil {
			return out, err
		}
		out.desc = int(binary.BigEndian.Uint32(raw[off:off+4]) & 0xffff)
		off += 4
	}
	if flags&0x000008 != 0 {
		if err := need(4); err != nil {
			return out, err
		}
		off += 4
	}
	if flags&0x000010 != 0 {
		if err := need(4); err != nil {
			return out, err
		}
		out.sampleSize = int64(binary.BigEndian.Uint32(raw[off : off+4]))
		off += 4
	}
	if flags&0x000020 != 0 {
		if err := need(4); err != nil {
			return out, err
		}
		off += 4
	}
	if out.desc < 1 {
		out.desc = 1
	}
	return out, nil
}

func (f *File) parseTRUN(box Box, defaultSize int64) (trunInfo, error) {
	raw, err := f.Payload(box, 0)
	if err != nil {
		return trunInfo{}, err
	}
	if len(raw) < 8 {
		return trunInfo{}, fmt.Errorf("trun too short")
	}
	flags := binary.BigEndian.Uint32(raw[0:4]) & 0x00ffffff
	count := binary.BigEndian.Uint32(raw[4:8])
	if count > 10_000_000 {
		return trunInfo{}, fmt.Errorf("trun sample count %d is implausible", count)
	}
	off := 8
	out := trunInfo{}
	if flags&0x000001 != 0 {
		if len(raw)-off < 4 {
			return out, fmt.Errorf("trun data offset truncated")
		}
		out.dataOffset = int64(int32(binary.BigEndian.Uint32(raw[off : off+4])))
		out.hasOffset = true
		off += 4
	}
	if flags&0x000004 != 0 {
		if len(raw)-off < 4 {
			return out, fmt.Errorf("trun first sample flags truncated")
		}
		off += 4
	}
	fields := 0
	if flags&0x000100 != 0 {
		fields++
	}
	if flags&0x000200 != 0 {
		fields++
	}
	if flags&0x000400 != 0 {
		fields++
	}
	if flags&0x000800 != 0 {
		fields++
	}
	if uint64(len(raw)-off) < uint64(count)*uint64(fields*4) {
		return out, fmt.Errorf("trun sample entries truncated")
	}
	out.sizes = make([]int64, count)
	for i := 0; i < int(count); i++ {
		if flags&0x000100 != 0 {
			off += 4
		}
		size := defaultSize
		if flags&0x000200 != 0 {
			size = int64(binary.BigEndian.Uint32(raw[off : off+4]))
			off += 4
		}
		if flags&0x000400 != 0 {
			off += 4
		}
		if flags&0x000800 != 0 {
			off += 4
		}
		if size < 0 {
			return out, fmt.Errorf("trun sample %d has negative size", i)
		}
		out.sizes[i] = size
	}
	return out, nil
}
