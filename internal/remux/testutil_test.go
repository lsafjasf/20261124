package remux

import (
	"hdr10remux/internal/ebml"
)

// blockPayload 组装 Block 数据区:轨道号 VINT + int16 相对时间戳 + 标志 + 数据。
func blockPayload(track uint64, rel int16, flags byte, data []byte) []byte {
	p := ebml.AppendSize(nil, track)
	p = append(p, byte(uint16(rel)>>8), byte(uint16(rel)))
	p = append(p, flags)
	return append(p, data...)
}

// xiphLace 生成 Xiph lacing 数据。
func xiphLace(frames [][]byte) []byte {
	data := []byte{byte(len(frames) - 1)}
	for _, f := range frames[:len(frames)-1] {
		n := len(f)
		for n >= 255 {
			data = append(data, 0xFF)
			n -= 255
		}
		data = append(data, byte(n))
	}
	for _, f := range frames {
		data = append(data, f...)
	}
	return data
}

// fixedLace 生成定长 lacing 数据(所有帧等长)。
func fixedLace(frames [][]byte) []byte {
	data := []byte{byte(len(frames) - 1)}
	for _, f := range frames {
		data = append(data, f...)
	}
	return data
}

// ebmlLace 生成 EBML lacing 数据(差值限于单字节范围)。
func ebmlLace(frames [][]byte) []byte {
	data := []byte{byte(len(frames) - 1)}
	data = ebml.AppendSize(data, uint64(len(frames[0])))
	for i := 1; i < len(frames)-1; i++ {
		diff := len(frames[i]) - len(frames[i-1])
		data = append(data, byte(diff+63)|0x80)
	}
	for _, f := range frames {
		data = append(data, f...)
	}
	return data
}

// 期望的全部帧(按解复用顺序):轨道号、时间戳、载荷。
var wantFrames = []struct {
	track uint64
	ts    int64
	data  string
}{
	{1, 0, "video-frame-0"},
	{2, 0, "aud0"}, {2, 0, "aud1"}, {2, 0, "aud2"},
	{1, 1000, "video-frame-1"},
	{2, 1010, "aa"}, {2, 1010, "bb"},
	{2, 1030, "x"}, {2, 1030, "yy"}, {2, 1030, "zzz"},
}
