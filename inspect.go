package tagfix

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type analysis struct {
	data   []byte
	top    []*mp4Box
	track  trackInfo
	colr   *colrInfo
	config *avcConfig
	sps    []*spsInfo
	mdat   []PayloadReport
}

type fieldSnapshot struct {
	Field      string `json:"field"`
	Value      any    `json:"value"`
	ByteOffset int64  `json:"byte_offset"`
	BitOffset  int    `json:"bit_offset,omitempty"`
}

type tagSource struct {
	Source string          `json:"source"`
	Tags   *ColorTags      `json:"tags,omitempty"`
	Fields []fieldSnapshot `json:"fields"`
}

// Inspection is the read-only view used to audit a file before a patch.
type Inspection struct {
	File       string          `json:"file,omitempty"`
	Track      int             `json:"track"`
	SampleType string          `json:"sample_entry"`
	Container  tagSource       `json:"container"`
	Bitstream  []tagSource     `json:"bitstream"`
	Payload    []PayloadReport `json:"media_payload"`
}

// PayloadReport proves that mdat bytes were copied without modification.
type PayloadReport struct {
	ByteOffset    int64  `json:"byte_offset"`
	NewByteOffset int64  `json:"new_byte_offset,omitempty"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	NewSHA256     string `json:"new_sha256,omitempty"`
	Unchanged     bool   `json:"unchanged"`
}

func analyze(data []byte, trackIndex int) (*analysis, error) {
	top, err := parseTopLevel(data)
	if err != nil {
		return nil, err
	}
	tracks, err := findVideoTracks(data, top)
	if err != nil {
		return nil, err
	}
	if trackIndex < 0 || trackIndex >= len(tracks) {
		return nil, fmt.Errorf("%w: video track %d does not exist (have %d)",
			ErrUnsupportedMedia, trackIndex, len(tracks))
	}
	track := tracks[trackIndex]
	result := &analysis{
		data: data, top: top, track: track, mdat: payloadReports(data, top),
	}
	if track.colr != nil {
		result.colr, err = parseColr(data, track.colr)
		if err != nil {
			return result, err
		}
	}
	if track.avcc == nil {
		return result, fmt.Errorf("%w: AVC track has no avcC/SPS to synchronize",
			ErrCannotAutoFix)
	}
	result.config, err = parseAVCConfig(data[track.avcc.payload:track.avcc.end])
	if err != nil {
		return result, err
	}
	for i, record := range result.config.sps {
		base := int64(track.avcc.payload + record.dataOffset)
		info, parseErr := parseSPS(record.nal, base)
		if parseErr != nil {
			return result, fmt.Errorf("SPS %d: %w", i, parseErr)
		}
		result.sps = append(result.sps, info)
	}
	return result, nil
}

func payloadReports(data []byte, top []*mp4Box) []PayloadReport {
	var reports []PayloadReport
	for _, box := range top {
		if box.typ != "mdat" {
			continue
		}
		payload := data[box.payload:box.end]
		digest := sha256.Sum256(payload)
		reports = append(reports, PayloadReport{
			ByteOffset: int64(box.payload),
			Size:       int64(len(payload)),
			SHA256:     hex.EncodeToString(digest[:]),
			Unchanged:  true,
		})
	}
	return reports
}

func (a *analysis) containerSource() tagSource {
	source := tagSource{Source: "container.colr"}
	if a.colr == nil || !a.colr.valid {
		return source
	}
	tags := a.colr.tags
	source.Tags = &tags
	source.Fields = []fieldSnapshot{
		{fieldPrimaries, tags.Primaries, int64(a.colr.primariesOffset + 1), 0},
		{fieldTransfer, tags.Transfer, int64(a.colr.transferOffset + 1), 0},
		{fieldMatrix, tags.Matrix, int64(a.colr.matrixOffset + 1), 0},
		{fieldFullRange, tags.FullRange, int64(a.colr.rangeOffset), 7},
	}
	return source
}

func (a *analysis) bitstreamSources() []tagSource {
	sources := make([]tagSource, 0, len(a.sps))
	for i, info := range a.sps {
		source := tagSource{
			Source: fmt.Sprintf("bitstream.avcC.sps[%d].vui", i),
		}
		if tags, complete := info.tags(); complete {
			source.Tags = &tags
		}
		for _, name := range []string{fieldPrimaries, fieldTransfer, fieldMatrix, fieldFullRange} {
			field, ok := info.fields[name]
			if !ok || !field.present {
				continue
			}
			offset, bitOffset, found := info.fieldOffset(name)
			if !found {
				continue
			}
			source.Fields = append(source.Fields, fieldSnapshot{
				Field: name, Value: field.value, ByteOffset: offset, BitOffset: bitOffset,
			})
		}
		sources = append(sources, source)
	}
	return sources
}

func (a *analysis) inspection() Inspection {
	return Inspection{
		Track:      a.track.index,
		SampleType: a.track.sampleType,
		Container:  a.containerSource(),
		Bitstream:  a.bitstreamSources(),
		Payload:    a.mdat,
	}
}

// InspectFile parses an MP4 and returns the current container and SPS tags.
func InspectFile(path string) (*Inspection, error) {
	return InspectFileTrack(path, 0)
}

// InspectFileTrack parses a specific zero-based AVC video track.
func InspectFileTrack(path string, trackIndex int) (*Inspection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return InspectTrack(data, trackIndex)
}

// Inspect parses an in-memory MP4.
func Inspect(data []byte) (*Inspection, error) {
	return InspectTrack(data, 0)
}

// InspectTrack parses an in-memory MP4 and selects a specific video track.
func InspectTrack(data []byte, trackIndex int) (*Inspection, error) {
	parsed, err := analyze(data, trackIndex)
	if err != nil {
		return nil, err
	}
	result := parsed.inspection()
	return &result, nil
}
