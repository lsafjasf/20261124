package ac3

import "sync"

// crcPoly is the ATSC A/52 generator polynomial x^16 + x^15 + x^2 + 1.
const crcPoly = 0x8005

var crcTable [256]uint16

func init() {
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ crcPoly
			} else {
				crc <<= 1
			}
		}
		crcTable[i] = crc
	}
}

// crc16 continues an AC-3 CRC-16 (init 0, MSB-first, no final XOR) over data.
func crc16(crc uint16, data []byte) uint16 {
	for _, b := range data {
		crc = crcTable[byte(crc>>8)^b] ^ (crc << 8)
	}
	return crc
}

// An AC-3 frame is split at the 5/8 point (q = 5*frameSize/8 bytes). The two
// CRC words are checked using the "zero-residue" convention, identical to
// FFmpeg/liba52 (AV_CRC_16_ANSI, init 0):
//
//   - CRC1 region: bytes [2, q), including the stored CRC1 word at [2,4);
//     a valid frame yields residue 0.
//   - CRC2 region: bytes [q, frameSize), including the stored CRC2 word in
//     the final two bytes; a valid frame yields residue 0.
//
// Because the sync word (bytes 0-1) is excluded from CRC1, the value that
// must be written into a CRC word is simply the CRC of its region computed
// with the word zeroed.

// crcResidues returns the raw CRC residue of both regions as stored. Both are
// zero on a valid frame.
func crcResidues(frame []byte) (crc1, crc2 uint16) {
	q := crcSplitByte(len(frame))
	crc1 = crc16(0, frame[2:q])
	crc2 = crc16(0, frame[q:])
	return crc1, crc2
}

// Inverse transforms depend only on region length, which is fixed per frame
// geometry; cache them so steady-state edits are pure table work.
var (
	crcInverseMu    sync.Mutex
	crcInverseCache = map[int]gfMatrix{}
)

func cachedInverse(regionLen, wordByteOffset int) gfMatrix {
	crcInverseMu.Lock()
	defer crcInverseMu.Unlock()
	key := regionLen*2 + wordByteOffset
	if inverse, ok := crcInverseCache[key]; ok {
		return inverse
	}
	inverse := buildCRCInverse(regionLen, wordByteOffset)
	crcInverseCache[key] = inverse
	return inverse
}

// frameCRCs computes the CRC words that must be stored after an edit. Each
// region is first evaluated with its own word zeroed; the inverse transform
// then yields the unique word that drives the region's residue back to zero.
func frameCRCs(frame []byte) (crc1, crc2 uint16) {
	n := len(frame)
	q := crcSplitByte(n)

	// CRC1: zero the word at [2,4), the region starts at byte 2.
	region1Len := q - 2
	region1 := make([]byte, region1Len)
	copy(region1, frame[2:q])
	region1[0], region1[1] = 0, 0
	crc1 = cachedInverse(region1Len, 0).multiply(crc16(0, region1))

	// CRC2: zero the final word of the frame, region starts at q.
	region2Len := n - q
	region2 := make([]byte, region2Len)
	copy(region2, frame[q:n])
	region2[region2Len-2], region2[region2Len-1] = 0, 0
	crc2 = cachedInverse(region2Len, region2Len-2).multiply(crc16(0, region2))
	return crc1, crc2
}
