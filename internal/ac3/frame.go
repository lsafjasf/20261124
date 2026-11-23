package ac3

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// SyncWord marks the start of every AC-3 sync frame: 0x0B77.
const SyncWord = 0x0b77

// DialnormMin/DialnormMax bound the 5-bit dialnorm field (ATSC A/52 5.4.2.5).
const (
	DialnormMin = 0
	DialnormMax = 31
)

var (
	ErrNotAC3          = errors.New("ac3: missing 0x0B77 sync word")
	ErrReservedFscod   = errors.New("ac3: reserved sample rate code fscod=3")
	ErrUnsupportedBsid = errors.New("ac3: bsid > 8 is not an AC-3 bitstream")
	ErrFrameTooShort   = errors.New("ac3: truncated frame")
)

// Frame describes one parsed AC-3 sync frame and the location of its fields.
type Frame struct {
	Data        []byte // exact frame bytes; length equals the A/52 frame size
	Index       int    // zero-based frame index within its stream
	Fscod       int    // sample rate code
	SampleRate  int
	Frmsizecod  int
	BitRateKbps int
	Bsid        int
	Bsmod       int
	Acmod       int
	Lfeon       int
	Dialnorm    int
	DialnormPos int // bit offset of the dialnorm field inside Data
	HasCompre   bool
	Compre      int
	Compr       int
}

// bitRateKbps derives the nominal bit rate from the frame-size code.
func bitRateKbps(frmsizecod int) int {
	nominal := []int{
		32, 32, 40, 40, 48, 48, 56, 56, 64, 64,
		80, 80, 96, 96, 112, 112, 128, 128, 160, 160,
		192, 192, 224, 224, 256, 256, 320, 320, 384, 384,
		448, 448, 512, 512, 576, 576, 640, 640,
	}
	if frmsizecod < 0 || frmsizecod >= len(nominal) {
		return -1
	}
	return nominal[frmsizecod]
}

// parseFrame parses one frame starting at data[0] and validates its CRCs.
func parseFrame(data []byte, index int) (*Frame, error) {
	if len(data) < 8 {
		return nil, ErrFrameTooShort
	}
	if int(data[0])<<8|int(data[1]) != SyncWord {
		return nil, ErrNotAC3
	}

	reader := newBitReader(data)
	syncword, _ := reader.bits(16)
	if syncword != SyncWord {
		return nil, ErrNotAC3
	}
	crc1Stored, _ := reader.bits(16) // bytes 2-3
	fscod, err := reader.bits(2)
	if err != nil {
		return nil, err
	}
	if fscod == 3 {
		return nil, ErrReservedFscod
	}
	frmsizecod, err := reader.bits(6)
	if err != nil {
		return nil, err
	}
	frameSize := frameSizeBytes(frmsizecod, fscod)
	if frameSize < 0 || len(data) < frameSize {
		return nil, fmt.Errorf("%w: need %d bytes, have %d", ErrFrameTooShort, frameSize, len(data))
	}

	// BSI (ATSC A/52 5.4.2). Only fields up to dialnorm are walked, which are
	// identical across every bsid <= 8 revision, so no tail parsing is needed
	// to locate the field we are allowed to edit.
	bsid, err := reader.bits(5)
	if err != nil {
		return nil, err
	}
	if bsid > 8 {
		return nil, fmt.Errorf("%w: bsid=%d", ErrUnsupportedBsid, bsid)
	}
	bsmod, err := reader.bits(3)
	if err != nil {
		return nil, err
	}
	acmod, err := reader.bits(3)
	if err != nil {
		return nil, err
	}

	// cmixlev: present whenever a center channel exists (odd acmod, A/52 5.4.2.3).
	if acmod&1 != 0 {
		if _, err := reader.bits(2); err != nil {
			return nil, err
		}
	}
	// surmixlev: present when a surround channel exists.
	if acmod&4 != 0 {
		if _, err := reader.bits(2); err != nil {
			return nil, err
		}
	}
	// dsurmod: present only for 2/0 mode.
	if acmod == 2 {
		if _, err := reader.bits(2); err != nil {
			return nil, err
		}
	}
	lfeon, err := reader.bits(1)
	if err != nil {
		return nil, err
	}
	dialnormPos := reader.pos
	dialnorm, err := reader.bits(5)
	if err != nil {
		return nil, err
	}
	compre, err := reader.bits(1)
	if err != nil {
		return nil, err
	}
	hasCompre := compre == 1
	compr := -1
	if hasCompre {
		compr, err = reader.bits(8)
		if err != nil {
			return nil, err
		}
	}

	frame := &Frame{
		Data:        data[:frameSize],
		Index:       index,
		Fscod:       fscod,
		SampleRate:  SampleRateCode[fscod],
		Frmsizecod:  frmsizecod,
		BitRateKbps: bitRateKbps(frmsizecod),
		Bsid:        bsid,
		Bsmod:       bsmod,
		Acmod:       acmod,
		Lfeon:       lfeon,
		Dialnorm:    dialnorm,
		DialnormPos: dialnormPos,
		HasCompre:   hasCompre,
		Compre:      compre,
		Compr:       compr,
	}

	residue1, residue2 := crcResidues(frame.Data)
	if residue1 != 0 {
		return nil, fmt.Errorf("ac3: frame %d CRC1 conflict: residue=0x%04X (stored word 0x%04X)",
			index, residue1, crc1Stored)
	}
	if residue2 != 0 {
		storedCRC2 := int(frame.Data[frameSize-2])<<8 | int(frame.Data[frameSize-1])
		return nil, fmt.Errorf("ac3: frame %d CRC2 conflict: residue=0x%04X (stored word 0x%04X)",
			index, residue2, storedCRC2)
	}
	return frame, nil
}

// payloadHash is the SHA-256 of the frame bytes, used to prove before/after
// equivalence outside of the explicitly permitted field plus CRC words.
func payloadHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
