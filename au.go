package au

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Magic is the four-byte Sun AU magic number, ".snd".
const Magic = ".snd"

const (
	// MinHeaderSize is the size of the fixed AU header.
	MinHeaderSize = 24
	// UnknownDataSize marks a header whose payload length is not declared.
	UnknownDataSize uint32 = 0xffffffff
)

// Encoding is the encoding field from an AU header.
type Encoding uint32

const (
	EncodingMuLaw    Encoding = 1
	EncodingLinear8  Encoding = 2
	EncodingLinear16 Encoding = 3
	EncodingLinear24 Encoding = 4
	EncodingLinear32 Encoding = 5
	EncodingFloat32  Encoding = 6
	EncodingFloat64  Encoding = 7
	EncodingALaw     Encoding = 27
	EncodingUnknown  Encoding = 0
)

func (e Encoding) String() string {
	switch e {
	case EncodingMuLaw:
		return "mu-law"
	case EncodingLinear8:
		return "linear-8"
	case EncodingLinear16:
		return "linear-16"
	case EncodingLinear24:
		return "linear-24"
	case EncodingLinear32:
		return "linear-32"
	case EncodingFloat32:
		return "float-32"
	case EncodingFloat64:
		return "float-64"
	case EncodingALaw:
		return "a-law"
	case EncodingUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("encoding-%d", uint32(e))
	}
}

// Kind groups the encodings this package can interpret as sample data.
type Kind uint8

const (
	KindUnknown Kind = iota
	KindMuLaw
	KindLinearPCM
)

func (k Kind) String() string {
	switch k {
	case KindMuLaw:
		return "mu-law"
	case KindLinearPCM:
		return "linear-pcm"
	default:
		return "unknown"
	}
}

// Header contains the fixed, big-endian AU header fields.
type Header struct {
	DataOffset uint32
	DataSize   uint32
	Encoding   Encoding
	SampleRate uint32
	Channels   uint32
}

// Format describes the byte layout of sample data.
type Format struct {
	Kind           Kind
	BitsPerSample  uint16
	BytesPerSample uint16
	BlockAlign     uint64
}

// File is the result of parsing an AU byte stream. Raw and Data are slices of
// the input passed to Parse; no header or sample byte is rewritten.
type File struct {
	Header Header
	Format Format

	// Raw is the complete original input.
	Raw []byte

	// Data contains only whole sample frames whose bytes are both present and
	// within the header's declared data size. It aliases Raw.
	Data []byte

	// DeclaredBytes is the usable payload length claimed by the header. When
	// the header uses UnknownDataSize, this is the number of bytes available
	// after DataOffset.
	DeclaredBytes uint64
	// DeclaredFrames is DeclaredBytes rounded down to a whole frame. For
	// unknown encodings it is zero because a frame size is not known.
	DeclaredFrames uint64
	// AvailableBytes is len(Raw) - DataOffset, clamped to zero.
	AvailableBytes uint64
	// OffsetPastEOF reports that DataOffset points beyond the input.
	OffsetPastEOF bool
	// AvailableFrames is the number of complete frames physically present
	// after DataOffset.
	AvailableFrames uint64
	// Frames is the number of complete frames retained in Data.
	Frames uint64

	// Truncated is true when fewer bytes are present than a known DataSize
	// declares.
	Truncated bool
	// SizeMismatch reports that a known DataSize differs from the number of
	// bytes physically present after DataOffset.
	SizeMismatch bool
	// IncompleteTail reports that physically present payload bytes end in the
	// middle of a sample frame.
	IncompleteTail bool
	// PartialBytes is the number of available bytes that cannot form one more
	// complete frame.
	PartialBytes uint64
	// ExtraBytes aliases the source and starts immediately after Data.
	ExtraBytes []byte
}

var (
	ErrShortHeader     = errors.New("au: short header")
	ErrBadMagic        = errors.New("au: bad magic")
	ErrInvalidOffset   = errors.New("au: data offset is inside the fixed header")
	ErrInvalidChannels = errors.New("au: channel count is zero")
	ErrUnknownEncoding = errors.New("au: unknown encoding")
)

// Parse parses data as a Sun AU stream. Unknown encoding IDs are accepted so
// callers can inspect the header and untouched payload; use Format.Kind to
// decide whether Samples is meaningful.
func Parse(data []byte) (*File, error) {
	if len(data) < MinHeaderSize {
		return nil, fmt.Errorf("%w: got %d bytes, need at least %d", ErrShortHeader, len(data), MinHeaderSize)
	}
	if string(data[:4]) != Magic {
		return nil, fmt.Errorf("%w: got %q", ErrBadMagic, data[:4])
	}

	h := Header{
		DataOffset: binary.BigEndian.Uint32(data[4:8]),
		DataSize:   binary.BigEndian.Uint32(data[8:12]),
		Encoding:   Encoding(binary.BigEndian.Uint32(data[12:16])),
		SampleRate: binary.BigEndian.Uint32(data[16:20]),
		Channels:   binary.BigEndian.Uint32(data[20:24]),
	}
	if h.DataOffset < MinHeaderSize {
		return nil, fmt.Errorf("%w: %d", ErrInvalidOffset, h.DataOffset)
	}
	if h.Channels == 0 {
		return nil, ErrInvalidChannels
	}

	f := &File{
		Header: h,
		Format: formatFor(h.Encoding),
		Raw:    data,
	}
	f.inspect()
	return f, nil
}

func formatFor(e Encoding) Format {
	switch e {
	case EncodingMuLaw:
		return Format{Kind: KindMuLaw, BitsPerSample: 8, BytesPerSample: 1}
	case EncodingLinear8:
		return Format{Kind: KindLinearPCM, BitsPerSample: 8, BytesPerSample: 1}
	case EncodingLinear16:
		return Format{Kind: KindLinearPCM, BitsPerSample: 16, BytesPerSample: 2}
	case EncodingLinear24:
		return Format{Kind: KindLinearPCM, BitsPerSample: 24, BytesPerSample: 3}
	case EncodingLinear32:
		return Format{Kind: KindLinearPCM, BitsPerSample: 32, BytesPerSample: 4}
	default:
		return Format{Kind: KindUnknown}
	}
}
