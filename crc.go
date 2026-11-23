package truehd

// The MLP/TrueHD checksums are not the usual MSB-first CRC16/CRC8 routines.
// They use FFmpeg's AVCRC table convention: the table is byte-swapped and the
// running value is folded with (value >> 8).

var crc16MLPTable [256]uint16
var crc8MLPTable [256]byte

func init() {
	for i := 0; i < 256; i++ {
		c := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if c&0x80000000 != 0 {
				c = (c << 1) ^ (uint32(0x2d) << 16)
			} else {
				c <<= 1
			}
		}
		crc16MLPTable[i] = uint16(bswap32(c))

		c = uint32(i) << 24
		for j := 0; j < 8; j++ {
			if c&0x80000000 != 0 {
				c = (c << 1) ^ (uint32(0x63) << 24)
			} else {
				c <<= 1
			}
		}
		crc8MLPTable[i] = byte(bswap32(c))
	}
}

func bswap32(v uint32) uint32 {
	return v>>24 | (v>>8)&0x0000ff00 | (v<<8)&0x00ff0000 | v<<24
}

func crc16MLP(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc = crc16MLPTable[byte(crc)^b] ^ (crc >> 8)
	}
	return crc
}

func crc8MLP(data []byte, crc byte) byte {
	for _, b := range data {
		crc = crc8MLPTable[crc^b]
	}
	return crc
}

func le16(data []byte) uint16 {
	return uint16(data[0]) | uint16(data[1])<<8
}

// validMajorSyncChecksum mirrors ff_mlp_checksum16(). The two bytes just
// before the stored checksum are folded in with XOR rather than CRC.
func validMajorSyncChecksum(header []byte) bool {
	if len(header) < 4 {
		return false
	}
	n := len(header)
	got := crc16MLP(header[:n-4]) ^ le16(header[n-4:n-2])
	return got == le16(header[n-2:n])
}

func xorBytes(data []byte) byte {
	var p byte
	for _, b := range data {
		p ^= b
	}
	return p
}

// validSubstreamCheck validates the two-byte data check at the end of a
// substream payload. TrueHD signals it with the checkdata_present flag.
func validSubstreamCheck(payload []byte) bool {
	if len(payload) < 3 {
		return false
	}
	n := len(payload) - 2
	wantParity := payload[n] ^ 0xa9
	if xorBytes(payload[:n]) != wantParity {
		return false
	}
	wantChecksum := crc8MLP(payload[:n-1], 0x3c) ^ payload[n-1]
	return payload[n+1] == wantChecksum
}
