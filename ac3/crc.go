package ac3

// AC-3 CRC-16: polynomial x^16 + x^15 + x^2 + 1 (0x8005), init 0x0000,
// MSB-first, no final xor (ATSC A/52 section 5.4.2).

var crcTable [256]uint16

func init() {
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x8005
			} else {
				crc <<= 1
			}
		}
		crcTable[i] = crc
	}
}

// CRC16 computes the AC-3 CRC over data.
func CRC16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc = (crc << 8) ^ crcTable[byte(crc>>8)^b]
	}
	return crc
}

// crc1Len returns the number of bytes covered by crc1: the first 5/8 of
// the frame, using the encoder formula (w>>1)+(w>>3) 16-bit words.
func crc1Len(frameBytes int) int {
	words := frameBytes / 2
	return 2 * ((words >> 1) + (words >> 3))
}

// checkCRC1 verifies crc1: encoders place crc1 so that the CRC of the
// first 5/8 of the frame, sync word excluded, is zero.
func checkCRC1(frame []byte) bool {
	n := crc1Len(len(frame))
	if n > len(frame) {
		return false
	}
	return CRC16(frame[2:n]) == 0
}

// checkCRC2 verifies crc2, which covers the whole frame (sync word
// excluded). With init 0 and no final xor, the CRC of the frame with the
// crc2 field included is 0.
func checkCRC2(frame []byte) bool {
	return CRC16(frame[2:]) == 0
}
