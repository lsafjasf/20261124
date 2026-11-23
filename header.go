package eac3sm

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const syncWord = 0x0B77

var (
	ErrNoSync      = errors.New("eac3sm: sync word not found")
	ErrNotEAC3     = errors.New("eac3sm: bsid out of E-AC-3 range")
	ErrShortFrame  = errors.New("eac3sm: frame shorter than declared frmsiz")
	ErrBadFscod    = errors.New("eac3sm: reserved sample-rate code")
	ErrCRCMismatch = errors.New("eac3sm: crc2 mismatch")
)

// Header carries the BSI fields of one E-AC-3 syncframe that the program
// state machine needs in order to (re)build a decoder configuration.
type Header struct {
	FrameType   int
	SubstreamID int
	FrameSize   int // total syncframe size in bytes, from frmsiz
	SampleRate  int // Hz
	NumBlocks   int // audio blocks per syncframe
	Acmod       int // raw audio coding mode
	LFE         bool
	BSID        int
	Dialnorm    int
	Compr       *int // compression gain word, nil when compre=0
	Dialnorm2   *int // present only for acmod==0 (1+1 dual mono)
	Compr2      *int
}

type bitReader struct {
	data []byte
	pos  int // bit position
}

func (r *bitReader) read(n int) (uint32, error) {
	if r.pos+n > len(r.data)*8 {
		return 0, ErrShortFrame
	}
	var v uint32
	for i := 0; i < n; i++ {
		v = v<<1 | uint32(r.data[r.pos>>3]>>(7-uint(r.pos&7))&1)
		r.pos++
	}
	return v, nil
}

var baseSampleRates = [3]int{48000, 44100, 32000}

// ParseHeader parses the fixed prefix of an E-AC-3 syncframe header.
// It does not verify the CRC; call ValidateCRC for that.
func ParseHeader(frame []byte) (*Header, error) {
	r := &bitReader{data: frame}
	read := func(n int) uint32 {
		v, err := r.read(n)
		if err != nil {
			panic(err)
		}
		return v
	}
	h := &Header{}
	err := func() (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = e.(error)
			}
		}()
		if read(16) != syncWord {
			return ErrNoSync
		}
		h.FrameType = int(read(2))
		h.SubstreamID = int(read(3))
		h.FrameSize = (int(read(11)) + 1) * 2
		fscod := int(read(2))
		if fscod == 3 {
			fscod2 := int(read(2))
			if fscod2 == 3 {
				return ErrBadFscod
			}
			h.SampleRate = baseSampleRates[fscod2] / 2
			h.NumBlocks = 6
		} else {
			h.SampleRate = baseSampleRates[fscod]
			numblkscod := int(read(2))
			h.NumBlocks = [4]int{1, 2, 3, 6}[numblkscod]
		}
		h.Acmod = int(read(3))
		h.LFE = read(1) == 1
		h.BSID = int(read(5))
		if h.BSID < 11 || h.BSID > 16 {
			return ErrNotEAC3
		}
		h.Dialnorm = int(read(5))
		if read(1) == 1 {
			v := int(read(8))
			h.Compr = &v
		}
		if h.Acmod == 0 { // 1+1 dual mono carries a second dialog/gain set
			v := int(read(5))
			h.Dialnorm2 = &v
			if read(1) == 1 {
				c := int(read(8))
				h.Compr2 = &c
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	if h.FrameSize > len(frame) {
		return nil, fmt.Errorf("%w: need %d bytes, have %d", ErrShortFrame, h.FrameSize, len(frame))
	}
	return h, nil
}

// ValidateCRC checks crc2, which covers the whole syncframe except its last
// two bytes.
func ValidateCRC(frame []byte, h *Header) bool {
	size := h.FrameSize
	if size < 4 || size > len(frame) {
		return false
	}
	want := binary.BigEndian.Uint16(frame[size-2 : size])
	return crc16(frame[:size-2]) == want
}
