package tagfix

import (
	"encoding/binary"
	"fmt"
)

type spsRecord struct {
	nal          []byte
	lengthOffset int
	dataOffset   int
	endOffset    int
}

type avcConfig struct {
	payload []byte
	sps     []spsRecord
}

func parseAVCConfig(payload []byte) (*avcConfig, error) {
	if len(payload) < 7 {
		return nil, fmt.Errorf("%w: avcC payload is too short", ErrUnsupportedMedia)
	}
	if payload[0] != 1 {
		return nil, fmt.Errorf("%w: avcC configurationVersion is %d", ErrUnsupportedMedia, payload[0])
	}
	count := int(payload[5] & 0x1f)
	if count == 0 {
		return nil, fmt.Errorf("%w: avcC contains no SPS", ErrCannotAutoFix)
	}
	config := &avcConfig{
		payload: append([]byte(nil), payload...),
		sps:     make([]spsRecord, 0, count),
	}
	pos := 6
	for i := 0; i < count; i++ {
		if pos+2 > len(payload) {
			return nil, fmt.Errorf("%w: truncated SPS length in avcC", ErrUnsupportedMedia)
		}
		length := int(binary.BigEndian.Uint16(payload[pos:]))
		start := pos + 2
		end := start + length
		if length == 0 || end > len(payload) {
			return nil, fmt.Errorf("%w: invalid SPS length %d in avcC", ErrUnsupportedMedia, length)
		}
		config.sps = append(config.sps, spsRecord{
			nal:          append([]byte(nil), payload[start:end]...),
			lengthOffset: pos,
			dataOffset:   start,
			endOffset:    end,
		})
		pos = end
	}
	if pos >= len(payload) {
		return nil, fmt.Errorf("%w: avcC has no PPS count", ErrUnsupportedMedia)
	}
	ppsCount := int(payload[pos])
	pos++
	for i := 0; i < ppsCount; i++ {
		if pos+2 > len(payload) {
			return nil, fmt.Errorf("%w: truncated PPS length in avcC", ErrUnsupportedMedia)
		}
		length := int(binary.BigEndian.Uint16(payload[pos:]))
		if length == 0 || pos+2+length > len(payload) {
			return nil, fmt.Errorf("%w: invalid PPS length %d in avcC", ErrUnsupportedMedia, length)
		}
		pos += 2 + length
	}
	return config, nil
}

func (c *avcConfig) rebuild(replacements [][]byte) ([]byte, error) {
	if len(replacements) != len(c.sps) {
		return nil, fmt.Errorf("%w: replacement count %d does not match SPS count %d",
			ErrCannotAutoFix, len(replacements), len(c.sps))
	}
	out := make([]byte, 0, len(c.payload))
	out = append(out, c.payload[:6]...)
	for i, record := range c.sps {
		nal := record.nal
		if len(replacements[i]) != 0 {
			nal = replacements[i]
		}
		if len(nal) == 0 || len(nal) > 0xffff {
			return nil, fmt.Errorf("%w: SPS %d has unsupported length %d",
				ErrCannotAutoFix, i, len(nal))
		}
		out = append(out, 0, 0)
		binary.BigEndian.PutUint16(out[len(out)-2:], uint16(len(nal)))
		out = append(out, nal...)
	}
	out = append(out, c.payload[c.sps[len(c.sps)-1].endOffset:]...)
	return out, nil
}
