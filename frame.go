package flacseek

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type frameHeader struct {
	Raw           []byte
	Size          int
	Blocking      byte
	Number        uint64
	BlockSize     uint16
	SampleRate    uint32
	Channels      uint8
	BitsPerSample uint8
}

func parseFrameHeader(r io.ReaderAt, off, size int64, info *StreamInfo) (frameHeader, error) {
	var hdr frameHeader
	n := int64(32)
	if remaining := size - off; remaining < n {
		n = remaining
	}
	if n < 6 {
		return hdr, io.ErrUnexpectedEOF
	}
	buf, err := readAt(r, off, int(n))
	if err != nil {
		return hdr, err
	}
	if buf[0] != 0xff || buf[1]&0xfe != 0xf8 {
		return hdr, errors.New("missing frame sync code")
	}
	if buf[1]&0x02 != 0 {
		return hdr, errors.New("reserved header bit is set")
	}
	hdr.Blocking = buf[1] & 0x01
	blockCode := buf[2] >> 4
	rateCode := buf[2] & 0x0f
	channelAssignment := buf[3] >> 4
	bpsCode := (buf[3] >> 1) & 0x07
	if buf[3]&0x01 != 0 {
		return hdr, errors.New("reserved header bit is set")
	}
	if blockCode == 0 {
		return hdr, errors.New("reserved block size code 0")
	}
	if rateCode == 15 {
		return hdr, errors.New("reserved sample rate code 15")
	}
	if channelAssignment > 10 {
		return hdr, fmt.Errorf("reserved channel assignment %d", channelAssignment)
	}
	if bpsCode == 3 {
		return hdr, errors.New("reserved sample size code 3")
	}

	number, numberLen, err := parseCodedNumber(buf[4:])
	if err != nil {
		return hdr, fmt.Errorf("invalid frame/sample number: %w", err)
	}
	hdr.Number = number
	pos := 4 + numberLen

	blockSize, extra, err := decodeBlockSize(blockCode, buf[pos:])
	if err != nil {
		return hdr, err
	}
	pos += extra
	hdr.BlockSize = blockSize
	if blockSize == 0 {
		return hdr, errors.New("frame block size is zero")
	}

	rate, extra, err := decodeSampleRate(rateCode, buf[pos:])
	if err != nil {
		return hdr, err
	}
	pos += extra
	if rateCode == 0 {
		rate = info.SampleRate
	}
	hdr.SampleRate = rate

	switch channelAssignment {
	case 0, 1, 2, 3, 4, 5, 6, 7:
		hdr.Channels = channelAssignment + 1
	case 8, 9, 10:
		hdr.Channels = 2
	}
	switch bpsCode {
	case 0:
		hdr.BitsPerSample = info.BitsPerSample
	case 1:
		hdr.BitsPerSample = 8
	case 2:
		hdr.BitsPerSample = 12
	case 4:
		hdr.BitsPerSample = 16
	case 5:
		hdr.BitsPerSample = 20
	case 6:
		hdr.BitsPerSample = 24
	case 7:
		hdr.BitsPerSample = 32
	}

	if pos+1 > len(buf) {
		return hdr, io.ErrUnexpectedEOF
	}
	if got, want := crc8(buf[:pos]), buf[pos]; got != want {
		return hdr, fmt.Errorf("header CRC-8 mismatch: got 0x%02x, want 0x%02x", want, got)
	}
	hdr.Size = pos + 1
	hdr.Raw = append([]byte(nil), buf[:hdr.Size]...)
	return hdr, nil
}

func parseCodedNumber(p []byte) (uint64, int, error) {
	if len(p) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	first := p[0]
	var n int
	var value uint64
	switch {
	case first&0x80 == 0:
		return uint64(first), 1, nil
	case first&0xe0 == 0xc0:
		n, value = 2, uint64(first&0x1f)
	case first&0xf0 == 0xe0:
		n, value = 3, uint64(first&0x0f)
	case first&0xf8 == 0xf0:
		n, value = 4, uint64(first&0x07)
	case first&0xfc == 0xf8:
		n, value = 5, uint64(first&0x03)
	case first&0xfe == 0xfc:
		n, value = 6, uint64(first&0x01)
	case first == 0xfe:
		n, value = 7, 0
	default:
		return 0, 0, errors.New("invalid leading byte")
	}
	if len(p) < n {
		return 0, 0, io.ErrUnexpectedEOF
	}
	for i := 1; i < n; i++ {
		if p[i]&0xc0 != 0x80 {
			return 0, 0, fmt.Errorf("invalid continuation byte at position %d", i)
		}
		value = value<<6 | uint64(p[i]&0x3f)
	}
	minValues := [...]uint64{0, 0, 0x80, 0x800, 0x10000, 0x200000, 0x4000000, 0x80000000}
	if value < minValues[n] {
		return 0, 0, errors.New("overlong coded number")
	}
	return value, n, nil
}

func decodeBlockSize(code byte, p []byte) (uint16, int, error) {
	switch {
	case code == 1:
		return 192, 0, nil
	case code >= 2 && code <= 5:
		return uint16(576 << (code - 2)), 0, nil
	case code == 6:
		if len(p) < 1 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint16(p[0]) + 1, 1, nil
	case code == 7:
		if len(p) < 2 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		v := binary.BigEndian.Uint16(p[:2])
		if v == 0xffff {
			return 0, 0, errors.New("block size 65536 exceeds the FLAC limit")
		}
		return v + 1, 2, nil
	case code >= 8:
		return uint16(256) << (code - 8), 0, nil
	default:
		return 0, 0, fmt.Errorf("reserved block size code %d", code)
	}
}

func decodeSampleRate(code byte, p []byte) (uint32, int, error) {
	switch code {
	case 0:
		return 0, 0, nil
	case 1:
		return 88200, 0, nil
	case 2:
		return 176400, 0, nil
	case 3:
		return 192000, 0, nil
	case 4:
		return 8000, 0, nil
	case 5:
		return 16000, 0, nil
	case 6:
		return 22050, 0, nil
	case 7:
		return 24000, 0, nil
	case 8:
		return 32000, 0, nil
	case 9:
		return 44100, 0, nil
	case 10:
		return 48000, 0, nil
	case 11:
		return 96000, 0, nil
	case 12:
		if len(p) < 1 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint32(p[0]) * 1000, 1, nil
	case 13:
		if len(p) < 2 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint32(binary.BigEndian.Uint16(p[:2])), 2, nil
	case 14:
		if len(p) < 2 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint32(binary.BigEndian.Uint16(p[:2])) * 10, 2, nil
	default:
		return 0, 0, fmt.Errorf("reserved sample rate code %d", code)
	}
}
