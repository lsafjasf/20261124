// Package mp3 scans MPEG-1/2 Layer III audio to estimate duration without
// decoding or altering any audio bytes.
package mp3

import "encoding/binary"

var bitrateTable = map[byte][15]uint32{
	// MPEG version 1, Layer III (version index 3)
	3: {0, 32000, 40000, 48000, 56000, 64000, 80000, 96000, 112000, 128000, 160000, 192000, 224000, 256000, 320000},
	// MPEG version 2/2.5, Layer III (version indices 2 and 0)
	2: {0, 8000, 16000, 24000, 32000, 40000, 48000, 56000, 64000, 80000, 96000, 112000, 128000, 144000, 160000},
	0: {0, 8000, 16000, 24000, 32000, 40000, 48000, 56000, 64000, 80000, 96000, 112000, 128000, 144000, 160000},
}

var sampleRateTable = map[byte][3]uint32{
	3: {44100, 48000, 32000},
	2: {22050, 24000, 16000},
	0: {11025, 12000, 8000},
}

type frameHeader struct {
	version    byte
	layer      byte
	bitrateIdx byte
	sampleIdx  byte
	padding    byte
	free       bool
	length     int
	samples    uint32
}

func parseHeader(h uint32) (frameHeader, bool) {
	if h>>21 != 0x7ff { // 11 sync bits
		return frameHeader{}, false
	}
	verBits := byte(h >> 19 & 0x3)
	layerBits := byte(h >> 17 & 0x3)
	brIdx := byte(h >> 12 & 0xf)
	srIdx := byte(h >> 10 & 0x3)
	pad := byte(h >> 9 & 0x1)
	if verBits == 1 || layerBits != 1 || brIdx == 0 || brIdx == 15 || srIdx == 3 {
		return frameHeader{}, false
	}
	version := map[byte]byte{0: 0, 2: 2, 3: 3}[verBits]
	fh := frameHeader{
		version:    version,
		layer:      3,
		bitrateIdx: brIdx,
		sampleIdx:  srIdx,
		padding:    pad,
		free:       brIdx == 0,
	}
	br := bitrateTable[version][brIdx-1]
	sr := sampleRateTable[version][srIdx]
	if version == 3 {
		fh.length = int((144*br+sr/2)/sr) + int(pad)
		fh.samples = 1152
	} else {
		fh.length = int((72*br+sr/2)/sr) + int(pad)
		fh.samples = 576
	}
	return fh, true
}

// DurationMillis returns the audio duration in milliseconds. It prefers an
// Xing/Info VBR header's exact frame count, falls back to a full frame scan,
// then to the supplied TLEN value.
func DurationMillis(audio []byte, tlenMillis uint32) (uint32, string) {
	if ms, ok := xingDuration(audio); ok {
		return ms, "xing"
	}
	if ms, ok := scanDuration(audio); ok {
		return ms, "frame-scan"
	}
	if tlenMillis > 0 {
		return tlenMillis, "tlen"
	}
	return 0, "unknown"
}

func xingDuration(audio []byte) (uint32, bool) {
	if len(audio) < 4 {
		return 0, false
	}
	h := binary.BigEndian.Uint32(audio[0:4])
	fh, ok := parseHeader(h)
	if !ok || fh.free || fh.length == 0 || fh.length > len(audio) {
		return 0, false
	}
	firstFrame := audio[:fh.length]
	off := 4 + 17 // side info is 17 bytes for MPEG1, 9 bytes for MPEG2/2.5
	if fh.version != 3 {
		off = 4 + 9
	}
	if firstFrame[1]&0x01 == 0 { // protection bit clear means a 16-bit CRC follows
		off += 2
	}
	if off+8 > len(audio) {
		return 0, false
	}
	tag := string(firstFrame[off : off+4])
	if tag != "Xing" && tag != "Info" {
		return 0, false
	}
	flags := binary.BigEndian.Uint32(firstFrame[off+4 : off+8])
	if flags&0x01 == 0 || off+12 > len(audio) {
		return 0, false
	}
	frames := binary.BigEndian.Uint32(firstFrame[off+8 : off+12])
	sr := sampleRateTable[fh.version][fh.sampleIdx]
	if frames == 0 || sr == 0 {
		return 0, false
	}
	return uint32(uint64(frames) * uint64(fh.samples) * 1000 / uint64(sr)), true
}

func scanDuration(audio []byte) (uint32, bool) {
	pos := 0
	var totalSamples uint64
	var sr uint32
	frames := 0
	for pos+4 <= len(audio) {
		fh, ok := parseHeader(binary.BigEndian.Uint32(audio[pos : pos+4]))
		if !ok {
			next := indexSync(audio, pos+1)
			if next < 0 {
				break
			}
			pos = next
			continue
		}
		if fh.free || fh.length <= 0 || pos+fh.length > len(audio) {
			next := indexSync(audio, pos+1)
			if next < 0 {
				break
			}
			pos = next
			continue
		}
		sr = sampleRateTable[fh.version][fh.sampleIdx]
		totalSamples += uint64(fh.samples)
		frames++
		pos += fh.length
	}
	if frames < 4 || sr == 0 {
		return 0, false
	}
	return uint32(totalSamples * 1000 / uint64(sr)), true
}

func indexSync(audio []byte, from int) int {
	for i := from; i+3 < len(audio); i++ {
		if audio[i] == 0xff && audio[i+1]&0xe0 == 0xe0 {
			if _, ok := parseHeader(binary.BigEndian.Uint32(audio[i : i+4])); ok {
				return i
			}
		}
	}
	return -1
}
