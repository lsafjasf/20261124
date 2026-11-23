package mp4

import (
	"encoding/binary"
	"fmt"

	"hdr10audit/bitstream"
	"hdr10audit/hdr"
)

type chunkRun struct {
	firstChunk int
	perChunk   int
	desc       int
}

func (f *File) parseSamples(children []Box, descriptions []Description) ([]Sample, error) {
	stsz := find(children, "stsz")
	stz2 := find(children, "stz2")
	if stsz == nil && stz2 == nil {
		return nil, fmt.Errorf("neither stsz nor stz2 found")
	}
	var sizes []int
	var err error
	if stsz != nil {
		sizes, err = f.sampleSizes(*stsz)
	} else {
		sizes, err = f.compactSampleSizes(*stz2)
	}
	if err != nil {
		return nil, err
	}
	if len(sizes) == 0 {
		return nil, nil
	}
	runs, err := f.chunkRuns(children)
	if err != nil {
		return nil, err
	}
	offsets, err := f.chunkOffsets(children)
	if err != nil {
		return nil, err
	}
	if len(offsets) == 0 {
		return nil, fmt.Errorf("sample sizes present but chunk offsets absent")
	}
	samples := make([]Sample, 0, len(sizes))
	runIndex := 0
	for chunk := 1; chunk <= len(offsets) && len(samples) < len(sizes); chunk++ {
		for runIndex+1 < len(runs) && runs[runIndex+1].firstChunk <= chunk {
			runIndex++
		}
		if len(runs) == 0 {
			return nil, fmt.Errorf("stsc has no entries")
		}
		desc := runs[runIndex].desc
		if desc < 1 || desc > len(descriptions) {
			return nil, fmt.Errorf("chunk %d references missing sample description %d", chunk, desc)
		}
		off := offsets[chunk-1]
		for n := 0; n < runs[runIndex].perChunk && len(samples) < len(sizes); n++ {
			size := sizes[len(samples)]
			if size < 0 {
				return nil, fmt.Errorf("sample %d has negative size", len(samples))
			}
			if off < 0 || off+int64(size) < off {
				return nil, fmt.Errorf("sample %d offset overflow", len(samples))
			}
			samples = append(samples, Sample{Offset: off, Size: int64(size), Description: desc})
			off += int64(size)
		}
	}
	if len(samples) != len(sizes) {
		return nil, fmt.Errorf("mapped %d samples, sample table declares %d", len(samples), len(sizes))
	}
	return samples, nil
}

func (f *File) sampleSizes(stsz Box) ([]int, error) {
	raw, err := f.Payload(stsz, 0)
	if err != nil {
		return nil, err
	}
	if len(raw) < 12 {
		return nil, fmt.Errorf("stsz too short")
	}
	fixed := binary.BigEndian.Uint32(raw[4:8])
	count := binary.BigEndian.Uint32(raw[8:12])
	if count > 10_000_000 {
		return nil, fmt.Errorf("stsz sample count %d is implausible", count)
	}
	sizes := make([]int, count)
	if fixed != 0 {
		for i := range sizes {
			sizes[i] = int(fixed)
		}
		return sizes, nil
	}
	if uint64(len(raw)) < 12+uint64(count)*4 {
		return nil, fmt.Errorf("stsz entries truncated")
	}
	for i := range sizes {
		sizes[i] = int(binary.BigEndian.Uint32(raw[12+i*4 : 16+i*4]))
	}
	return sizes, nil
}

func (f *File) compactSampleSizes(stz2 Box) ([]int, error) {
	raw, err := f.Payload(stz2, 0)
	if err != nil {
		return nil, err
	}
	if len(raw) < 12 {
		return nil, fmt.Errorf("stz2 too short")
	}
	fieldSize := int(raw[7])
	count := binary.BigEndian.Uint32(raw[8:12])
	if count > 10_000_000 {
		return nil, fmt.Errorf("stz2 sample count %d is implausible", count)
	}
	sizes := make([]int, count)
	data := raw[12:]
	switch fieldSize {
	case 4:
		if len(data)*2 < int(count) {
			return nil, fmt.Errorf("stz2 4-bit entries truncated")
		}
		for i := range sizes {
			if i%2 == 0 {
				sizes[i] = int(data[i/2] >> 4)
			} else {
				sizes[i] = int(data[i/2] & 0x0f)
			}
		}
	case 8:
		if len(data) < int(count) {
			return nil, fmt.Errorf("stz2 8-bit entries truncated")
		}
		for i := range sizes {
			sizes[i] = int(data[i])
		}
	case 16:
		if len(data) < int(count)*2 {
			return nil, fmt.Errorf("stz2 16-bit entries truncated")
		}
		for i := range sizes {
			sizes[i] = int(binary.BigEndian.Uint16(data[i*2 : i*2+2]))
		}
	default:
		return nil, fmt.Errorf("invalid stz2 field size %d", fieldSize)
	}
	return sizes, nil
}

func (f *File) chunkRuns(children []Box) ([]chunkRun, error) {
	stsc := find(children, "stsc")
	if stsc == nil {
		return nil, fmt.Errorf("stsc not found")
	}
	raw, err := f.Payload(*stsc, 0)
	if err != nil {
		return nil, err
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("stsc too short")
	}
	count := binary.BigEndian.Uint32(raw[4:8])
	if uint64(len(raw)) < 8+uint64(count)*12 {
		return nil, fmt.Errorf("stsc entries truncated")
	}
	runs := make([]chunkRun, 0, count)
	last := 0
	for i := 0; i < int(count); i++ {
		p := raw[8+i*12:]
		first := int(binary.BigEndian.Uint32(p[0:4]))
		perChunk := int(binary.BigEndian.Uint32(p[4:8]))
		desc := int(binary.BigEndian.Uint32(p[8:12]))
		if first <= last || first < 1 || perChunk <= 0 || desc < 1 {
			return nil, fmt.Errorf("invalid stsc entry %d: first=%d count=%d desc=%d",
				i, first, perChunk, desc)
		}
		runs = append(runs, chunkRun{firstChunk: first, perChunk: perChunk, desc: desc})
		last = first
	}
	return runs, nil
}

func (f *File) chunkOffsets(children []Box) ([]int64, error) {
	if stco := find(children, "stco"); stco != nil {
		raw, err := f.Payload(*stco, 0)
		if err != nil {
			return nil, err
		}
		if len(raw) < 8 {
			return nil, fmt.Errorf("stco too short")
		}
		count := binary.BigEndian.Uint32(raw[4:8])
		if uint64(len(raw)) < 8+uint64(count)*4 {
			return nil, fmt.Errorf("stco entries truncated")
		}
		offsets := make([]int64, count)
		for i := range offsets {
			offsets[i] = int64(binary.BigEndian.Uint32(raw[8+i*4 : 12+i*4]))
		}
		return offsets, nil
	}
	if co64 := find(children, "co64"); co64 != nil {
		raw, err := f.Payload(*co64, 0)
		if err != nil {
			return nil, err
		}
		if len(raw) < 8 {
			return nil, fmt.Errorf("co64 too short")
		}
		count := binary.BigEndian.Uint32(raw[4:8])
		if uint64(len(raw)) < 8+uint64(count)*8 {
			return nil, fmt.Errorf("co64 entries truncated")
		}
		offsets := make([]int64, count)
		for i := range offsets {
			offsets[i] = int64(binary.BigEndian.Uint64(raw[8+i*8 : 16+i*8]))
		}
		return offsets, nil
	}
	return nil, fmt.Errorf("stco/co64 not found")
}

func (f *File) scanTrackBitstream(track *Track) error {
	for i, sample := range track.Samples {
		if sample.Description < 1 || sample.Description > len(track.Descriptions) {
			track.Issues = append(track.Issues, hdr.Issue{
				Code: "sample_description_missing", Severity: hdr.SeverityError,
				Message: fmt.Sprintf("sample %d references description %d", i, sample.Description),
			})
			continue
		}
		d := track.Descriptions[sample.Description-1]
		codec := codecForEntry(d.Type)
		if codec == 0 || d.LengthSize == 0 {
			continue
		}
		raw, err := f.ReadAt(sample.Offset, sample.Size)
		if err != nil {
			track.Issues = append(track.Issues, hdr.Issue{
				Code: "sample_read_failed", Severity: hdr.SeverityError,
				Location: fmt.Sprintf("sample[%d]", i), Message: err.Error(),
			})
			continue
		}
		nals, err := bitstream.SplitNALUnits(raw, d.LengthSize)
		if err != nil {
			track.Issues = append(track.Issues, hdr.Issue{
				Code: "malformed_sample", Severity: hdr.SeverityError,
				Location: fmt.Sprintf("sample[%d]", i), Message: err.Error(),
			})
			continue
		}
		for j, nal := range nals {
			r := bitstream.ScanNAL(nal, codec, fmt.Sprintf("bitstream:sample[%d]/nal[%d]", i, j))
			track.Issues = append(track.Issues, r.Issues...)
			mergeOccurrences(&track.Bitstream, r.Occurrences)
		}
	}
	return nil
}

func codecForEntry(typ string) bitstream.Codec {
	switch typ {
	case "hvc1", "hev1":
		return bitstream.HEVC
	case "avc1", "avc3":
		return bitstream.AVC
	default:
		return 0
	}
}

func mergeOccurrences(dst *[]hdr.Occurrence, src []hdr.Occurrence) {
	for _, in := range src {
		if in.Count < 1 {
			in.Count = 1
		}
		merged := false
		for i := range *dst {
			if occurrencesEqual((*dst)[i], in) {
				(*dst)[i].Count += in.Count
				merged = true
				break
			}
		}
		if !merged {
			*dst = append(*dst, in)
		}
	}
}

func occurrencesEqual(a, b hdr.Occurrence) bool {
	if a.Source != b.Source || (a.Metadata.Mastering == nil) != (b.Metadata.Mastering == nil) ||
		(a.Metadata.Light == nil) != (b.Metadata.Light == nil) {
		return false
	}
	if a.Metadata.Mastering != nil && *a.Metadata.Mastering != *b.Metadata.Mastering {
		return false
	}
	if a.Metadata.Light != nil && *a.Metadata.Light != *b.Metadata.Light {
		return false
	}
	return true
}
