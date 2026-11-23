package flacseek

import (
	"encoding/binary"
	"testing"
)

type testFrameSpec struct {
	sampleStart uint64
	blockSize   uint16
	variable    bool
	frameNumber uint64
	payloadByte byte
}

type testSeekPoint struct {
	sample uint64
	offset uint64
	count  uint16
}

func buildTestFLAC(t *testing.T, specs []testFrameSpec, seek []testSeekPoint) []byte {
	t.Helper()
	audio := buildTestAudio(t, specs)
	total := uint64(0)
	minBlock, maxBlock := specs[0].blockSize, specs[0].blockSize
	for _, spec := range specs {
		total += uint64(spec.blockSize)
		if spec.blockSize < minBlock {
			minBlock = spec.blockSize
		}
		if spec.blockSize > maxBlock {
			maxBlock = spec.blockSize
		}
	}
	streamInfo := makeStreamInfo(minBlock, maxBlock, 44100, 1, 16, total)
	out := []byte("fLaC")
	if seek == nil {
		out = appendMetadata(out, 0, true, streamInfo)
	} else {
		out = appendMetadata(out, 0, false, streamInfo)
		out = appendMetadata(out, 3, true, encodeTestSeek(seek))
	}
	out = append(out, audio...)
	return out
}

func buildTestAudio(t *testing.T, specs []testFrameSpec) []byte {
	t.Helper()
	var audio []byte
	for i, spec := range specs {
		spec.frameNumber = uint64(i)
		audio = append(audio, buildTestFrame(spec)...)
	}
	return audio
}

func buildTestFrame(spec testFrameSpec) []byte {
	blockCode, extra := testBlockSize(spec.blockSize)
	blocking := byte(0)
	number := uint64(0)
	if spec.variable {
		blocking = 1
		number = spec.sampleStart
	} else {
		number = spec.frameNumber
	}
	header := []byte{0xff, 0xf8 | blocking, blockCode<<4 | 9, 0x08}
	header = append(header, encodeTestNumber(number)...)
	header = append(header, extra...)
	header = append(header, crc8(header))

	payload := []byte{0x00, spec.payloadByte, 0x00}
	frame := append(append([]byte(nil), header...), payload...)
	crc := crc16(frame)
	frame = append(frame, byte(crc>>8), byte(crc))
	return frame
}

func testBlockSize(size uint16) (byte, []byte) {
	switch size {
	case 192:
		return 1, nil
	case 576:
		return 2, nil
	case 1152:
		return 3, nil
	case 2304:
		return 4, nil
	case 4608:
		return 5, nil
	case 256:
		return 8, nil
	case 512:
		return 9, nil
	case 1024:
		return 10, nil
	case 2048:
		return 11, nil
	case 4096:
		return 12, nil
	case 8192:
		return 13, nil
	case 16384:
		return 14, nil
	case 32768:
		return 15, nil
	default:
		if size <= 256 {
			return 6, []byte{byte(size - 1)}
		}
		return 7, []byte{byte((size - 1) >> 8), byte(size - 1)}
	}
}

func encodeTestNumber(v uint64) []byte {
	if v < 0x80 {
		return []byte{byte(v)}
	}
	n := 2
	switch {
	case v < 1<<11:
		n = 2
	case v < 1<<16:
		n = 3
	case v < 1<<21:
		n = 4
	case v < 1<<26:
		n = 5
	case v < 1<<31:
		n = 6
	default:
		n = 7
	}
	out := make([]byte, n)
	firstMask := byte(0xff << (8 - n))
	out[0] = firstMask | byte(v>>(6*uint(n-1)))
	for i := 1; i < n; i++ {
		out[i] = 0x80 | byte(v>>(6*uint(n-1-i)))&0x3f
	}
	return out
}

func makeStreamInfo(minBlock, maxBlock uint16, rate uint32, channels, bps uint8, total uint64) []byte {
	data := make([]byte, 34)
	binary.BigEndian.PutUint16(data[0:2], minBlock)
	binary.BigEndian.PutUint16(data[2:4], maxBlock)
	v := uint64(rate)<<44 | uint64(channels-1)<<41 | uint64(bps-1)<<36 | total
	binary.BigEndian.PutUint64(data[10:18], v)
	return data
}

func appendMetadata(dst []byte, blockType byte, last bool, data []byte) []byte {
	head := []byte{blockType, byte(len(data) >> 16), byte(len(data) >> 8), byte(len(data))}
	if last {
		head[0] |= 0x80
	}
	dst = append(dst, head...)
	return append(dst, data...)
}

func encodeTestSeek(points []testSeekPoint) []byte {
	data := make([]byte, len(points)*18)
	for i, point := range points {
		entry := data[i*18 : (i+1)*18]
		binary.BigEndian.PutUint64(entry[0:8], point.sample)
		binary.BigEndian.PutUint64(entry[8:16], point.offset)
		binary.BigEndian.PutUint16(entry[16:18], point.count)
	}
	return data
}

func variableSpecs() []testFrameSpec {
	return []testFrameSpec{
		{sampleStart: 0, blockSize: 192, variable: true, payloadByte: 1},
		{sampleStart: 192, blockSize: 576, variable: true, payloadByte: 2},
		{sampleStart: 768, blockSize: 1000, variable: true, payloadByte: 3},
		{sampleStart: 1768, blockSize: 4096, variable: true, payloadByte: 4},
	}
}

func validSeekForAudio(specs []testFrameSpec) []testSeekPoint {
	points := make([]testSeekPoint, len(specs))
	offset := uint64(0)
	for i, spec := range specs {
		points[i] = testSeekPoint{sample: spec.sampleStart, offset: offset, count: spec.blockSize}
		offset += uint64(len(buildTestFrame(spec)))
	}
	return points
}
