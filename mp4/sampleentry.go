package mp4

import (
	"encoding/binary"
	"fmt"

	"hdr10audit/bitstream"
	"hdr10audit/hdr"
)

func (f *File) parseDescriptions(track *Track, stsd Box) error {
	if stsd.Size-stsd.HeaderSize < 8 {
		return fmt.Errorf("stsd too short")
	}
	head, err := f.ReadAt(stsd.PayloadStart, 8)
	if err != nil {
		return err
	}
	declared := binary.BigEndian.Uint32(head[4:8])
	entries, err := f.Boxes(stsd.PayloadStart+8, stsd.End())
	if err != nil {
		return fmt.Errorf("parse stsd entries: %w", err)
	}
	if int(declared) != len(entries) {
		track.Issues = append(track.Issues, hdr.Issue{
			Code: "stsd_entry_count_mismatch", Severity: hdr.SeverityWarning,
			Message: fmt.Sprintf("stsd 声明 %d 个条目，实际解析到 %d 个", declared, len(entries)),
		})
	}
	for i, entry := range entries {
		d := Description{Index: i + 1, Type: entry.Type}
		if isVideoEntry(entry.Type) {
			if err := f.parseVideoSampleEntry(&d, entry); err != nil {
				track.Issues = append(track.Issues, hdr.Issue{
					Code: "malformed_sample_entry", Severity: hdr.SeverityError,
					Location: fmt.Sprintf("container:stsd[%d]/%s", d.Index, entry.Type),
					Message:  err.Error(),
				})
			}
		}
		track.Descriptions = append(track.Descriptions, d)
		for _, occ := range d.ConfigMetadata {
			track.Bitstream = append(track.Bitstream, occ)
		}
	}
	return nil
}

func isVideoEntry(typ string) bool {
	switch typ {
	case "hvc1", "hev1", "avc1", "avc3":
		return true
	default:
		return false
	}
}

func (f *File) parseVideoSampleEntry(d *Description, entry Box) error {
	if entry.Size-entry.HeaderSize < 78 {
		return fmt.Errorf("visual sample entry is %d bytes, want at least 78", entry.Size-entry.HeaderSize)
	}
	children, err := f.Boxes(entry.PayloadStart+78, entry.End())
	if err != nil {
		return fmt.Errorf("parse sample entry children: %w", err)
	}
	for _, child := range children {
		switch child.Type {
		case "hvcC":
			raw, err := f.Payload(child, 16<<20)
			if err != nil {
				return err
			}
			lengthSize, nals, err := parseHEVCC(raw)
			if err != nil {
				return fmt.Errorf("hvcC: %w", err)
			}
			d.LengthSize = lengthSize
			d.ConfigNALs = nals
			d.ConfigMetadata = scanConfigNALs(nals, bitstream.HEVC,
				fmt.Sprintf("bitstream:hvcC/stsd[%d]", d.Index))
		case "avcC":
			raw, err := f.Payload(child, 16<<20)
			if err != nil {
				return err
			}
			lengthSize, nals, err := parseAVCC(raw)
			if err != nil {
				return fmt.Errorf("avcC: %w", err)
			}
			d.LengthSize = lengthSize
			d.ConfigNALs = nals
			d.ConfigMetadata = scanConfigNALs(nals, bitstream.AVC,
				fmt.Sprintf("bitstream:avcC/stsd[%d]", d.Index))
		case "mdcv", "clli":
			occ, box, err := f.parseMetadataBox(child, d)
			if err != nil {
				return err
			}
			d.Metadata = append(d.Metadata, occ)
			d.MetadataBox = append(d.MetadataBox, box)
		}
	}
	return nil
}

func scanConfigNALs(nals [][]byte, codec bitstream.Codec, base string) []hdr.Occurrence {
	var out []hdr.Occurrence
	for i, nal := range nals {
		r := bitstream.ScanNAL(nal, codec, fmt.Sprintf("%s/nal[%d]", base, i))
		out = append(out, r.Occurrences...)
	}
	return out
}

func (f *File) parseMetadataBox(box Box, d *Description) (hdr.Occurrence, MetadataBox, error) {
	raw, err := f.Payload(box, 64)
	if err != nil {
		return hdr.Occurrence{}, MetadataBox{}, err
	}
	loc := fmt.Sprintf("container:stsd[%d]/%s/%s", d.Index, d.Type, box.Type)
	occ := hdr.Occurrence{Source: hdr.SourceContainer, Location: loc, Count: 1}
	switch box.Type {
	case "mdcv":
		m, err := hdr.DecodeMastering(raw)
		if err != nil {
			return occ, MetadataBox{}, err
		}
		occ.Metadata.Mastering = &m
	case "clli":
		c, err := hdr.DecodeContentLight(raw)
		if err != nil {
			return occ, MetadataBox{}, err
		}
		occ.Metadata.Light = &c
	}
	mb := MetadataBox{
		Type: box.Type, PayloadStart: box.PayloadStart,
		PayloadSize: box.Size - box.HeaderSize, Occurrence: occ,
	}
	return occ, mb, nil
}

func parseHEVCC(data []byte) (int, [][]byte, error) {
	if len(data) < 23 {
		return 0, nil, fmt.Errorf("payload is %d bytes, want at least 23", len(data))
	}
	lengthSize := int(data[21]&3) + 1
	numArrays := int(data[22])
	off := 23
	var nals [][]byte
	for i := 0; i < numArrays; i++ {
		if len(data)-off < 3 {
			return 0, nil, fmt.Errorf("array %d header truncated", i)
		}
		numNALUs := int(binary.BigEndian.Uint16(data[off+1 : off+3]))
		off += 3
		for j := 0; j < numNALUs; j++ {
			if len(data)-off < 2 {
				return 0, nil, fmt.Errorf("array %d NAL %d length truncated", i, j)
			}
			n := int(binary.BigEndian.Uint16(data[off : off+2]))
			off += 2
			if n > len(data)-off {
				return 0, nil, fmt.Errorf("array %d NAL %d length %d exceeds remaining %d",
					i, j, n, len(data)-off)
			}
			nals = append(nals, data[off:off+n])
			off += n
		}
	}
	if off != len(data) {
		return 0, nil, fmt.Errorf("%d trailing bytes after NAL arrays", len(data)-off)
	}
	return lengthSize, nals, nil
}

func parseAVCC(data []byte) (int, [][]byte, error) {
	if len(data) < 7 {
		return 0, nil, fmt.Errorf("payload is %d bytes, want at least 7", len(data))
	}
	lengthSize := int(data[4]&3) + 1
	off := 6
	var nals [][]byte
	numSPS := int(data[5] & 0x1f)
	for i := 0; i < numSPS; i++ {
		if len(data)-off < 2 {
			return 0, nil, fmt.Errorf("SPS %d length truncated", i)
		}
		n := int(binary.BigEndian.Uint16(data[off : off+2]))
		off += 2
		if n > len(data)-off {
			return 0, nil, fmt.Errorf("SPS %d length %d exceeds remaining %d", i, n, len(data)-off)
		}
		nals = append(nals, data[off:off+n])
		off += n
	}
	if len(data)-off < 1 {
		return 0, nil, fmt.Errorf("PPS count missing")
	}
	numPPS := int(data[off])
	off++
	for i := 0; i < numPPS; i++ {
		if len(data)-off < 2 {
			return 0, nil, fmt.Errorf("PPS %d length truncated", i)
		}
		n := int(binary.BigEndian.Uint16(data[off : off+2]))
		off += 2
		if n > len(data)-off {
			return 0, nil, fmt.Errorf("PPS %d length %d exceeds remaining %d", i, n, len(data)-off)
		}
		nals = append(nals, data[off:off+n])
		off += n
	}
	return lengthSize, nals, nil
}
