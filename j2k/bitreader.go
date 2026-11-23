package j2k

import "errors"

var errTruncatedBits = errors.New("truncated packet header")

// bitReader reads JPEG 2000 packet-header bits. After an 0xFF byte the next
// byte carries only seven data bits; its most significant bit is stuffed.
type bitReader struct {
	data     []byte
	start    int
	end      int
	pos      int
	current  byte
	bitsLeft int
	lastByte byte
	loaded   bool
}

func newBitReader(data []byte, start, end int) *bitReader {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	return &bitReader{data: data, start: start, end: end, pos: start}
}

func (r *bitReader) fill() error {
	if r.pos >= r.end {
		return errTruncatedBits
	}
	b := r.data[r.pos]
	r.pos++
	r.bitsLeft = 8
	if r.loaded && r.lastByte == 0xff {
		r.bitsLeft = 7
	}
	r.current = b
	r.lastByte = b
	r.loaded = true
	return nil
}

func (r *bitReader) readBits(n int) (uint64, error) {
	if n < 0 || n > 32 {
		return 0, errors.New("invalid bit count")
	}
	var v uint64
	for n > 0 {
		if r.bitsLeft == 0 {
			if err := r.fill(); err != nil {
				return v, err
			}
		}
		take := n
		if take > r.bitsLeft {
			take = r.bitsLeft
		}
		shift := r.bitsLeft - take
		mask := uint32(1<<take) - 1
		v = (v << take) | uint64((uint32(r.current)>>shift)&mask)
		r.bitsLeft -= take
		n -= take
	}
	return v, nil
}

func (r *bitReader) readBit() (int, error) {
	v, err := r.readBits(1)
	return int(v), err
}

// align finishes a packet header. If the last physical byte was 0xFF, its
// stuffed successor belongs to the packet header too.
func (r *bitReader) align() error {
	if r.loaded && r.lastByte == 0xff {
		if r.pos >= r.end {
			return errTruncatedBits
		}
		r.pos++
	}
	r.bitsLeft = 0
	return nil
}

func (r *bitReader) consumed() int {
	return r.pos - r.start
}
