package ac3

// A CRC word must make its region's zero-residue CRC equal zero. The mapping
// from a 16-bit word (placed at a fixed offset inside the region) to the
// resulting CRC is linear over GF(2), so it can be inverted once per region
// length and reused as a constant-time matrix multiplication.

// gfMatrix is a 16x16 GF(2) matrix stored as 16 row bitmasks (bit 15 = MSB).
type gfMatrix [16]uint16

// invert builds the GF(2) inverse of the forward transform that maps a word
// at wordByteOffset inside a region of regionLen bytes to its CRC residue.
func buildCRCInverse(regionLen, wordByteOffset int) gfMatrix {
	// Forward matrix column in: CRC contribution when one input word bit is set.
	var forward [16]uint16
	for in := 0; in < 16; in++ {
		buf := make([]byte, regionLen)
		word := uint16(1 << uint(15-in))
		buf[wordByteOffset] = byte(word >> 8)
		buf[wordByteOffset+1] = byte(word)
		forward[in] = crc16(0, buf)
	}

	// Translate column contributions into output-bit rows, then augment with
	// the identity matrix and run Gauss-Jordan elimination to obtain the
	// inverse in the augmented half.
	augmented := make([]uint32, 16)
	for out := 0; out < 16; out++ {
		var rowMask uint16
		for in := 0; in < 16; in++ {
			if forward[in]&(1<<uint(15-out)) != 0 {
				rowMask |= 1 << uint(15-in)
			}
		}
		augmented[out] = uint32(rowMask) | uint32(1<<uint(15-out))<<16
	}
	for col := 0; col < 16; col++ {
		pivot := -1
		for r := col; r < 16; r++ {
			if augmented[r]&(1<<uint(15-col)) != 0 {
				pivot = r
				break
			}
		}
		augmented[col], augmented[pivot] = augmented[pivot], augmented[col]
		for r := 0; r < 16; r++ {
			if r != col && augmented[r]&(1<<uint(15-col)) != 0 {
				augmented[r] ^= augmented[col]
			}
		}
	}

	var inverse gfMatrix
	for i := range inverse {
		inverse[i] = uint16(augmented[i] >> 16)
	}
	return inverse
}

// multiply returns matrix * vector over GF(2).
func (m gfMatrix) multiply(vector uint16) uint16 {
	var result uint16
	for row := 0; row < 16; row++ {
		parity := vector & m[row]
		// Population parity via the standard fold.
		parity ^= parity >> 8
		parity ^= parity >> 4
		parity ^= parity >> 2
		parity ^= parity >> 1
		if parity&1 != 0 {
			result |= 1 << uint(15-row)
		}
	}
	return result
}
