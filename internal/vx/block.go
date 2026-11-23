package vx

import (
	"encoding/binary"
	"fmt"
)

const frameHeaderSize = 17

// flags bit 0 = keyframe, bit 1 = scene cut.
func decodeFrame(payload []byte, plane int) (*Frame, error) {
	if len(payload) < frameHeaderSize {
		return nil, fmt.Errorf("payload %d bytes shorter than frame header", len(payload))
	}
	fr := &Frame{
		Num: binary.LittleEndian.Uint32(payload[0:4]),
		PTS: int64(binary.LittleEndian.Uint64(payload[4:12])),
	}
	flags := payload[12]
	fr.Key = flags&1 != 0
	fr.Cut = flags&2 != 0
	planes := payload[frameHeaderSize:]
	if len(planes) != 3*plane {
		return nil, fmt.Errorf("frame %d payload planes=%d, want %d", fr.Num, len(planes), 3*plane)
	}
	fr.Y = append([]byte(nil), planes[:plane]...)
	fr.U = append([]byte(nil), planes[plane:2*plane]...)
	fr.V = append([]byte(nil), planes[2*plane:]...)
	return fr, nil
}

func decodeAudio(payload []byte) (*AudioPacket, error) {
	if len(payload) < 10 {
		return nil, fmt.Errorf("audio payload %d bytes too short", len(payload))
	}
	p := &AudioPacket{
		SampleStart: binary.LittleEndian.Uint64(payload[0:8]),
	}
	frameCount := binary.LittleEndian.Uint16(payload[8:10])
	body := payload[10:]
	if len(body) != int(frameCount)*2 {
		return nil, fmt.Errorf("audio frames=%d but body=%d bytes", frameCount, len(body))
	}
	p.Samples = make([]int16, frameCount)
	for i := range p.Samples {
		p.Samples[i] = int16(binary.LittleEndian.Uint16(body[2*i:]))
	}
	return p, nil
}
