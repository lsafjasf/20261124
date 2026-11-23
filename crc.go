package flacseek

// crc8 computes the FLAC header CRC-8 (polynomial 0x07, initial value 0).
func crc8(p []byte) byte {
	var crc byte
	for _, b := range p {
		crc ^= b
		for range 8 {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// crc16 computes the FLAC frame CRC-16 (polynomial 0x8005, initial value 0).
func crc16(p []byte) uint16 {
	var crc uint16
	for _, b := range p {
		crc = crc16Update(crc, b)
	}
	return crc
}

func crc16Update(crc uint16, b byte) uint16 {
	crc ^= uint16(b) << 8
	for range 8 {
		if crc&0x8000 != 0 {
			crc = crc<<1 ^ 0x8005
		} else {
			crc <<= 1
		}
	}
	return crc
}
