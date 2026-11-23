package eac3sm

// crc16 implements the CRC-16 used by E-AC-3 (ATSC A/52 Annex E):
// polynomial x^16 + x^15 + x^2 + 1 (0x8005), MSB first, initial value 0.
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
