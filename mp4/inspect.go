package mp4

import (
	"fmt"
	"strings"

	"hdr10audit/hdr"
)

type Range struct {
	Start int64 `json:"start"`
	Size  int64 `json:"size"`
}

type Sample struct {
	Offset      int64
	Size        int64
	Description int
}

type MetadataBox struct {
	Type         string
	PayloadStart int64
	PayloadSize  int64
	Occurrence   hdr.Occurrence
}

type Description struct {
	Index          int
	Type           string
	LengthSize     int
	ConfigNALs     [][]byte
	ConfigMetadata []hdr.Occurrence
	Metadata       []hdr.Occurrence
	MetadataBox    []MetadataBox
}

type Track struct {
	ID           uint32
	Handler      string
	Codec        string
	Descriptions []Description
	Samples      []Sample
	Container    []hdr.Occurrence
	Bitstream    []hdr.Occurrence
	Issues       []hdr.Issue
}

type Info struct {
	Tracks    []Track
	MediaData []Range
	Issues    []hdr.Issue
}

func (f *File) Inspect() (*Info, error) {
	top, err := f.TopLevel()
	if err != nil {
		return nil, err
	}
	info := &Info{}
	for _, b := range top {
		if b.Type == "mdat" {
			info.MediaData = append(info.MediaData, Range{
				Start: b.PayloadStart,
				Size:  b.Size - b.HeaderSize,
			})
		}
	}
	moov := find(top, "moov")
	if moov == nil {
		return nil, fmt.Errorf("moov box not found")
	}
	children, err := f.Children(*moov)
	if err != nil {
		return nil, fmt.Errorf("parse moov: %w", err)
	}
	fragmented := len(findAll(top, "moof")) > 0
	for _, trak := range findAll(children, "trak") {
		track, err := f.inspectTrack(trak, fragmented)
		if err != nil {
			return nil, fmt.Errorf("parse trak at %d: %w", trak.Start, err)
		}
		if track.Handler == "vide" {
			info.Tracks = append(info.Tracks, *track)
		}
	}
	if len(info.Tracks) == 0 {
		return nil, fmt.Errorf("no video track found")
	}
	if fragmented {
		if err := f.appendFragments(info, top); err != nil {
			return nil, err
		}
	}
	for i := range info.Tracks {
		f.scanTrackBitstream(&info.Tracks[i])
	}
	return info, nil
}

func (f *File) inspectTrack(trak Box, allowFragmented bool) (*Track, error) {
	track := &Track{}
	children, err := f.Children(trak)
	if err != nil {
		return nil, err
	}
	if tkhd := find(children, "tkhd"); tkhd != nil {
		track.ID, _ = f.trackID(*tkhd)
	}
	mdia := find(children, "mdia")
	if mdia == nil {
		return nil, fmt.Errorf("mdia not found")
	}
	mdiaChildren, err := f.Children(*mdia)
	if err != nil {
		return nil, err
	}
	if hdlr := find(mdiaChildren, "hdlr"); hdlr != nil {
		track.Handler, _ = f.handlerType(*hdlr)
	}
	minf := find(mdiaChildren, "minf")
	if minf == nil {
		return nil, fmt.Errorf("minf not found")
	}
	minfChildren, err := f.Children(*minf)
	if err != nil {
		return nil, err
	}
	stbl := find(minfChildren, "stbl")
	if stbl == nil {
		return nil, fmt.Errorf("stbl not found")
	}
	stblChildren, err := f.Children(*stbl)
	if err != nil {
		return nil, err
	}
	if stsd := find(stblChildren, "stsd"); stsd != nil {
		if err := f.parseDescriptions(track, *stsd); err != nil {
			return nil, err
		}
	} else {
		track.Issues = append(track.Issues, hdr.Issue{
			Code: "missing_stsd", Severity: hdr.SeverityError,
			Message: "stsd 缺失，无法识别视频编码",
		})
	}
	hasSampleTable := (find(stblChildren, "stsz") != nil || find(stblChildren, "stz2") != nil) &&
		find(stblChildren, "stsc") != nil &&
		(find(stblChildren, "stco") != nil || find(stblChildren, "co64") != nil)
	if hasSampleTable {
		samples, err := f.parseSamples(stblChildren, track.Descriptions)
		if err != nil {
			return nil, err
		}
		track.Samples = samples
	} else if !allowFragmented {
		return nil, fmt.Errorf("sample table is incomplete")
	}
	for _, d := range track.Descriptions {
		track.Container = append(track.Container, d.Metadata...)
	}
	var codecs []string
	for _, d := range track.Descriptions {
		codecs = append(codecs, d.Type)
	}
	track.Codec = strings.Join(codecs, ",")
	return track, nil
}

func (f *File) trackID(box Box) (uint32, error) {
	raw, err := f.Payload(box, 64)
	if err != nil {
		return 0, err
	}
	if len(raw) < 16 {
		return 0, fmt.Errorf("tkhd too short")
	}
	off := 12
	if raw[0] == 1 {
		off = 20
	}
	if len(raw) < off+4 {
		return 0, fmt.Errorf("tkhd too short for track ID")
	}
	return uint32(raw[off])<<24 | uint32(raw[off+1])<<16 |
		uint32(raw[off+2])<<8 | uint32(raw[off+3]), nil
}

func (f *File) handlerType(box Box) (string, error) {
	raw, err := f.Payload(box, 64)
	if err != nil {
		return "", err
	}
	if len(raw) < 12 {
		return "", fmt.Errorf("hdlr too short")
	}
	return string(raw[8:12]), nil
}
