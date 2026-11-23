package tagfix

import "fmt"

type bitReader struct {
	data []byte
	pos  uint64
}

func (r *bitReader) readBit() (uint64, error) {
	if r.pos >= uint64(len(r.data))*8 {
		return 0, fmt.Errorf("%w: truncated bitstream at bit %d", ErrUnsupportedMedia, r.pos)
	}
	value := uint64((r.data[r.pos/8] >> (7 - r.pos%8)) & 1)
	r.pos++
	return value, nil
}

func (r *bitReader) readBits(count int) (uint64, error) {
	if count < 0 || count > 64 {
		return 0, fmt.Errorf("%w: invalid bit count %d", ErrUnsupportedMedia, count)
	}
	var value uint64
	for i := 0; i < count; i++ {
		bit, err := r.readBit()
		if err != nil {
			return 0, err
		}
		value = value<<1 | bit
	}
	return value, nil
}

func (r *bitReader) readUE() (uint64, error) {
	zeros := 0
	for {
		bit, err := r.readBit()
		if err != nil {
			return 0, err
		}
		if bit == 1 {
			break
		}
		zeros++
		if zeros > 31 {
			return 0, fmt.Errorf("%w: Exp-Golomb value is too large", ErrUnsupportedMedia)
		}
	}
	if zeros == 0 {
		return 0, nil
	}
	tail, err := r.readBits(zeros)
	if err != nil {
		return 0, err
	}
	return (uint64(1)<<zeros - 1) + tail, nil
}

func (r *bitReader) readSE() (int64, error) {
	value, err := r.readUE()
	if err != nil {
		return 0, err
	}
	if value&1 == 1 {
		return int64(value/2) + 1, nil
	}
	return -int64(value / 2), nil
}

func bytesToBits(data []byte) []byte {
	bits := make([]byte, len(data)*8)
	for i, value := range data {
		for bit := 0; bit < 8; bit++ {
			bits[i*8+bit] = (value >> (7 - bit)) & 1
		}
	}
	return bits
}

func bitsToBytes(bits []byte) []byte {
	out := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		out[i/8] |= bit << (7 - i%8)
	}
	return out
}

// trimRBSP keeps the stop bit and drops old byte-alignment zeros. The caller
// then re-pads to a byte boundary after inserting bits.
func trimRBSP(bits []byte) []byte {
	lastOne := -1
	for i, bit := range bits {
		if bit != 0 {
			lastOne = i
		}
	}
	if lastOne < 0 {
		return nil
	}
	return bits[:lastOne+1]
}

func setBit(bits []byte, pos uint64, value byte) error {
	if pos >= uint64(len(bits)) {
		return fmt.Errorf("%w: bit position %d is out of range", ErrCannotAutoFix, pos)
	}
	bits[pos] = value & 1
	return nil
}

func insertBits(bits []byte, pos uint64, inserted []byte) ([]byte, error) {
	if pos > uint64(len(bits)) {
		return nil, fmt.Errorf("%w: insertion bit %d is out of range", ErrCannotAutoFix, pos)
	}
	out := make([]byte, 0, len(bits)+len(inserted))
	out = append(out, bits[:pos]...)
	out = append(out, inserted...)
	out = append(out, bits[pos:]...)
	return out, nil
}

func appendBits(dst []byte, value uint64, count int) []byte {
	for i := count - 1; i >= 0; i-- {
		dst = append(dst, byte((value>>i)&1))
	}
	return dst
}
