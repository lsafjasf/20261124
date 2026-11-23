package pcm

import (
	"encoding/binary"
	"errors"
	"math"
)

// ErrBadPTS is returned when a PTS chunk payload cannot be decoded.
var ErrBadPTS = errors.New("pcm: malformed PTS payload")

// Sample is one decoded record inside a PTS chunk: an opaque geometry id plus
// its anchor coordinates. The geometry id is what ADM objects reference.
type Sample struct {
	Ref string
	X   float64
	Y   float64
	Z   float64
}

// EncodePTS serializes samples into a PTS chunk payload:
// repeated [uint32 refLen][ref bytes][float64 x][float64 y][float64 z].
func EncodePTS(samples []Sample) []byte {
	size := 0
	for _, s := range samples {
		size += 4 + len(s.Ref) + 24
	}
	out := make([]byte, 0, size)
	for _, s := range samples {
		out = binary.LittleEndian.AppendUint32(out, uint32(len(s.Ref)))
		out = append(out, s.Ref...)
		out = binary.LittleEndian.AppendUint64(out, math.Float64bits(s.X))
		out = binary.LittleEndian.AppendUint64(out, math.Float64bits(s.Y))
		out = binary.LittleEndian.AppendUint64(out, math.Float64bits(s.Z))
	}
	return out
}

// ParsePTS decodes a PTS chunk payload.
func ParsePTS(data []byte) ([]Sample, error) {
	var samples []Sample
	for offset := 0; offset < len(data); {
		if offset+4 > len(data) {
			return nil, ErrBadPTS
		}
		refLen := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if refLen == 0 || offset+refLen+24 > len(data) {
			return nil, ErrBadPTS
		}
		ref := string(data[offset : offset+refLen])
		offset += refLen
		sample := Sample{Ref: ref}
		sample.X = math.Float64frombits(binary.LittleEndian.Uint64(data[offset : offset+8]))
		sample.Y = math.Float64frombits(binary.LittleEndian.Uint64(data[offset+8 : offset+16]))
		sample.Z = math.Float64frombits(binary.LittleEndian.Uint64(data[offset+16 : offset+24]))
		offset += 24
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		return nil, ErrBadPTS
	}
	return samples, nil
}

// GeometryRefs collects every geometry reference id across all PTS chunks.
func (f *File) GeometryRefs() (map[string]struct{}, error) {
	refs := make(map[string]struct{})
	for _, chunk := range f.Chunks {
		if chunk.Type != ChunkPTS {
			continue
		}
		samples, err := ParsePTS(chunk.Data)
		if err != nil {
			return nil, err
		}
		for _, sample := range samples {
			refs[sample.Ref] = struct{}{}
		}
	}
	if len(refs) == 0 {
		return nil, ErrBadPTS
	}
	return refs, nil
}
