package au

import (
	"encoding/binary"
	"fmt"
	"math"
)

// DecodeMuLaw expands one G.711 μ-law byte to a signed 16-bit sample.
func DecodeMuLaw(v byte) int16 {
	v = ^v
	t := int16(((int(v)&0x0f)<<3 + 0x84) << ((v & 0x70) >> 4))
	if v&0x80 != 0 {
		return 0x84 - t
	}
	return t - 0x84
}

// Samples decodes all complete frames in Data into interleaved signed samples.
// μ-law values are expanded to 16-bit; 8/16/24/32-bit PCM uses sign extension.
// The source bytes are only read, never changed.
func (f *File) Samples() ([]int32, error) {
	if f.Format.Kind == KindUnknown || f.Format.BlockAlign == 0 {
		return nil, fmt.Errorf("%w: encoding %d", ErrUnknownEncoding, f.Header.Encoding)
	}
	count := uint64(len(f.Data)) / f.Format.BlockAlign * uint64(f.Header.Channels)
	if count > uint64(math.MaxInt) {
		return nil, fmt.Errorf("au: sample count %d exceeds platform limit", count)
	}

	out := make([]int32, 0, int(count))
	switch f.Header.Encoding {
	case EncodingMuLaw:
		for _, b := range f.Data {
			out = append(out, int32(DecodeMuLaw(b)))
		}
	case EncodingLinear8:
		for _, b := range f.Data {
			out = append(out, int32(int8(b)))
		}
	case EncodingLinear16:
		for i := 0; i+2 <= len(f.Data); i += 2 {
			out = append(out, int32(int16(binary.BigEndian.Uint16(f.Data[i:]))))
		}
	case EncodingLinear24:
		for i := 0; i+3 <= len(f.Data); i += 3 {
			v := int32(f.Data[i])<<16 | int32(f.Data[i+1])<<8 | int32(f.Data[i+2])
			if v&0x800000 != 0 {
				v -= 1 << 24
			}
			out = append(out, v)
		}
	case EncodingLinear32:
		for i := 0; i+4 <= len(f.Data); i += 4 {
			out = append(out, int32(binary.BigEndian.Uint32(f.Data[i:])))
		}
	default:
		return nil, fmt.Errorf("%w: encoding %d", ErrUnknownEncoding, f.Header.Encoding)
	}
	return out, nil
}
