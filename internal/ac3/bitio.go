package ac3

import "errors"

// bitReader consumes AC-3 big-endian, MSB-first bits.
type bitReader struct {
	data []byte
	pos  int // bit position
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (r *bitReader) bits(n int) (int, error) {
	if n < 0 || r.pos+n > len(r.data)*8 {
		return 0, errors.New("ac3: bit read past end of frame")
	}
	value := 0
	for i := 0; i < n; i++ {
		byteIndex := (r.pos + i) >> 3
		bitIndex := 7 - ((r.pos + i) & 7)
		value = value<<1 | int((r.data[byteIndex]>>uint(bitIndex))&1)
	}
	r.pos += n
	return value, nil
}

// writeBits overwrites n MSB-first bits at the given bit position without
// touching any neighboring bit, so permitted-field edits stay bit-accurate.
func writeBits(data []byte, bitPos, n, value int) error {
	if n <= 0 || bitPos < 0 || bitPos+n > len(data)*8 {
		return errors.New("ac3: bit write out of range")
	}
	if value < 0 || value >= 1<<uint(n) {
		return errors.New("ac3: value does not fit in field width")
	}
	for i := 0; i < n; i++ {
		bit := (value >> uint(n-1-i)) & 1
		byteIndex := (bitPos + i) >> 3
		bitIndex := uint(7 - ((bitPos + i) & 7))
		if bit == 1 {
			data[byteIndex] |= 1 << bitIndex
		} else {
			data[byteIndex] &^= 1 << bitIndex
		}
	}
	return nil
}

// readBitsAt reads n bits at an absolute position without mutating reader state.
func readBitsAt(data []byte, bitPos, n int) (int, error) {
	reader := newBitReader(data)
	reader.pos = bitPos
	return reader.bits(n)
}
