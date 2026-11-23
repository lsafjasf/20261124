package ac3

import "fmt"

const SyncWord = 0x0B77

// Frame holds the metadata parsed from one AC-3 syncframe.
type Frame struct {
	Index      int   // 0-based frame number in the stream
	Offset     int64 // byte offset of the sync word in the stream
	FrameBytes int   // total frame size in bytes

	FSCod      int
	FrmSizeCod int
	SampleRate int
	BitRate    int // kbps
	BSID       int
	BSMod      int
	ACMod      int
	LFEOn      bool
	Dialnorm   int // 1..31, decoder attenuation in dB

	ComprPresent bool
	Compr        int // signed 8-bit line-mode DRC value
	DynrngSet    bool
	Dynrng       int // signed 8-bit RF-mode DRC value from audio block 0

	CRC1OK bool
	CRC2OK bool
}

// ChannelConfig returns a human readable channel layout, e.g. "3/2+LFE".
func (f *Frame) ChannelConfig() string {
	s := acmodTable[f.ACMod].label
	if f.LFEOn {
		s += "+LFE"
	}
	return s
}

// BSModText describes the bitstream mode (service type).
func (f *Frame) BSModText() string { return bsmodTable[f.BSMod] }

// LoudnessLKFS estimates program loudness from dialnorm: dialogue sits at
// -dialnorm dBFS, so program loudness is approximately -dialnorm LKFS.
func (f *Frame) LoudnessLKFS() float64 { return -float64(f.Dialnorm) }

// ComprGainDB decodes the compr value to a gain in dB (value/32).
func (f *Frame) ComprGainDB() float64 { return float64(f.Compr) / 32 }

// DynrngGainDB decodes the dynrng value to a gain in dB (value/32).
func (f *Frame) DynrngGainDB() float64 { return float64(f.Dynrng) / 32 }

type bitReader struct {
	data []byte
	pos  int // bit position
}

func (r *bitReader) u(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v <<= 1
		if r.pos>>3 < len(r.data) {
			v |= int(r.data[r.pos>>3]>>(7-uint(r.pos&7))) & 1
		}
		r.pos++
	}
	return v
}

// FrameSize inspects a buffer starting at a sync word and returns the
// total frame size in bytes. It needs at least 5 bytes.
func FrameSize(buf []byte) (int, error) {
	if len(buf) < 5 {
		return 0, fmt.Errorf("need at least 5 bytes, have %d", len(buf))
	}
	if buf[0] != 0x0B || buf[1] != 0x77 {
		return 0, fmt.Errorf("no sync word")
	}
	fscod := int(buf[4] >> 6)
	frmsizecod := int(buf[4] & 0x3F)
	if fscod > 2 {
		return 0, fmt.Errorf("reserved fscod %d", fscod)
	}
	if frmsizecod > 37 {
		return 0, fmt.Errorf("invalid frmsizecod %d", frmsizecod)
	}
	return frameSizeTable[frmsizecod][fscod] * 2, nil
}

// ParseFrame parses one complete syncframe (len(data) must cover the
// whole frame as reported by FrameSize).
func ParseFrame(data []byte, index int, offset int64) (*Frame, error) {
	size, err := FrameSize(data)
	if err != nil {
		return nil, err
	}
	if len(data) < size {
		return nil, fmt.Errorf("truncated frame: need %d bytes, have %d", size, len(data))
	}
	data = data[:size]

	f := &Frame{Index: index, Offset: offset, FrameBytes: size}
	f.CRC1OK = checkCRC1(data)
	f.CRC2OK = checkCRC2(data)

	r := &bitReader{data: data, pos: 32} // skip syncword + crc1
	f.FSCod = r.u(2)
	f.FrmSizeCod = r.u(6)
	f.SampleRate = sampleRateTable[f.FSCod]
	f.BitRate = bitRateTable[f.FrmSizeCod>>1]
	f.BSID = r.u(5)
	if f.BSID > 8 {
		return nil, fmt.Errorf("bsid %d: not an AC-3 frame (E-AC-3?)", f.BSID)
	}
	f.BSMod = r.u(3)
	f.ACMod = r.u(3)
	info := acmodTable[f.ACMod]
	if f.ACMod&1 != 0 && f.ACMod != 1 {
		r.u(2) // cmixlev
	}
	if f.ACMod&4 != 0 {
		r.u(2) // surmixlev
	}
	if f.ACMod == 2 {
		r.u(2) // dsurmod
	}
	f.LFEOn = r.u(1) == 1
	f.Dialnorm = r.u(5)
	if r.u(1) == 1 { // compre
		f.ComprPresent = true
		f.Compr = int(int8(r.u(8)))
	}
	if f.ACMod == 0 { // 1+1 mode carries a second language code
		if r.u(1) == 1 {
			r.u(8) // langcod
		}
	}
	if r.u(1) == 1 { // audprodie
		r.u(5) // mixlevel
		r.u(2) // roomtyp
	}
	if f.ACMod == 0 {
		if r.u(1) == 1 { // audprodi2e
			r.u(5)
			r.u(2)
		}
	}
	r.u(1)           // copyrightb
	r.u(1)           // origbs
	if r.u(1) == 1 { // timecod1e
		r.u(14)
	}
	if r.u(1) == 1 { // timecod2e
		r.u(14)
	}
	if r.u(1) == 1 { // addbsie
		addbsil := r.u(6)
		r.u((addbsil + 1) * 8)
	}

	// Audio block 0: skip blksw and dithflag per channel, then dynrng.
	nfch := info.nfchans
	r.u(nfch)        // blksw
	r.u(nfch)        // dithflag
	if r.u(1) == 1 { // dynrnge
		f.DynrngSet = true
		f.Dynrng = int(int8(r.u(8)))
	}
	return f, nil
}
