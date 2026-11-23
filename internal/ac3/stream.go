package ac3

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// Stream is a validated sequence of AC-3 frames sharing one byte buffer.
type Stream struct {
	Frames []*Frame
}

// ParseStream splits a raw AC-3 elementary stream into CRC-checked frames.
// Sync words are located defensively rather than trusted, so a corrupt frame
// size cannot make the scan silently lose data; trailing bytes are rejected.
func ParseStream(data []byte) (*Stream, error) {
	stream := &Stream{}
	offset := 0
	index := 0
	for offset < len(data) {
		if offset+2 > len(data) || int(data[offset])<<8|int(data[offset+1]) != SyncWord {
			return nil, fmt.Errorf("ac3: expected sync word at byte %d", offset)
		}
		frame, err := parseFrame(data[offset:], index)
		if err != nil {
			return nil, err
		}
		stream.Frames = append(stream.Frames, frame)
		offset += len(frame.Data)
		index++
	}
	if len(stream.Frames) == 0 {
		return nil, errors.New("ac3: stream contains no frames")
	}
	return stream, nil
}

// FrameHash returns the SHA-256 of one frame's current bytes.
func (s *Stream) FrameHash(index int) string {
	return payloadHash(s.Frames[index].Data)
}

// FileHash returns the SHA-256 of the reassembled stream.
func (s *Stream) FileHash() string {
	sum := sha256.Sum256(s.Bytes())
	return hex.EncodeToString(sum[:])
}

// Bytes reassembles the stream by concatenating every frame in order.
func (s *Stream) Bytes() []byte {
	total := 0
	for _, frame := range s.Frames {
		total += len(frame.Data)
	}
	out := make([]byte, 0, total)
	for _, frame := range s.Frames {
		out = append(out, frame.Data...)
	}
	return out
}

// SetDialnorm rewrites only the 5 dialnorm bits of one frame, then recomputes
// both CRC words. It refuses out-of-range values and verifies afterwards that
// no byte other than the dialnorm byte and the two CRC words changed.
func SetDialnorm(frame *Frame, target int) (changed bool, err error) {
	if target < DialnormMin || target > DialnormMax {
		return false, fmt.Errorf("ac3: dialnorm %d out of range [%d,%d]", target, DialnormMin, DialnormMax)
	}
	if target == frame.Dialnorm {
		return false, nil
	}

	before := make([]byte, len(frame.Data))
	copy(before, frame.Data)

	if err := writeBits(frame.Data, frame.DialnormPos, 5, target); err != nil {
		return false, err
	}
	newCRC1, newCRC2 := frameCRCs(frame.Data)
	frame.Data[2] = byte(newCRC1 >> 8)
	frame.Data[3] = byte(newCRC1)
	frame.Data[len(frame.Data)-2] = byte(newCRC2 >> 8)
	frame.Data[len(frame.Data)-1] = byte(newCRC2)

	if err := verifyEditMask(before, frame.Data, frame); err != nil {
		return false, err
	}
	if check1, check2 := frameCRCs(frame.Data); check1 != newCRC1 || check2 != newCRC2 {
		return false, errors.New("ac3: recomputed CRC verification conflict")
	}
	frame.Dialnorm = target
	return true, nil
}

// verifyEditMask enforces the bit-level contract: between before and after, the
// only differing bits live in dialnorm itself and the two 16-bit CRC words.
func verifyEditMask(before, after []byte, frame *Frame) error {
	if len(before) != len(after) {
		return errors.New("ac3: frame length changed during edit")
	}
	allowed := make([]byte, len(before))
	// dialnorm is 5 MSB-first bits and may straddle two bytes.
	for bit := frame.DialnormPos; bit < frame.DialnormPos+5; bit++ {
		allowed[bit/8] |= 1 << uint(7-bit%8)
	}
	allowed[2] = 0xFF
	allowed[3] = 0xFF
	allowed[len(before)-2] = 0xFF
	allowed[len(before)-1] = 0xFF

	for i := range before {
		unexpected := (before[i] ^ after[i]) &^ allowed[i]
		if unexpected != 0 {
			return fmt.Errorf("ac3: illegal bit change at byte %d (mask 0x%02X)", i, unexpected)
		}
	}
	return nil
}

// FramesEqual reports whether two frames are byte-for-byte identical, which is
// how duplicated splice frames at ad/program boundaries are detected.
func FramesEqual(a, b *Frame) bool {
	return bytes.Equal(a.Data, b.Data)
}
