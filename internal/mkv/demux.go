package mkv

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"hdr10remux/internal/ebml"
)

// Frame 是一帧音视频载荷(已解 lacing)。
type Frame struct {
	TrackNumber uint64
	// Timestamp 是相对 Segment 的时间戳,单位为 TimestampScale。
	Timestamp int64
	Keyframe  bool
	Payload   []byte
}

// Cluster 是一个簇及其包含的全部帧。
type Cluster struct {
	// Timestamp 是簇时间戳,单位为 TimestampScale。
	Timestamp uint64
	Frames    []Frame
}

// Demuxer 以流式方式逐个产出 Cluster。
type Demuxer struct {
	r      io.ReadSeeker
	info   *SourceInfo
	segEnd int64
}

// NewDemuxer 基于已解析的头信息创建解复用器。
func NewDemuxer(r io.ReadSeeker, info *SourceInfo) *Demuxer {
	return &Demuxer{r: r, info: info, segEnd: info.SegmentEnd}
}

// Next 读取下一个 Cluster;结束时返回 io.EOF。
func (d *Demuxer) Next() (*Cluster, error) {
	for {
		pos, err := d.r.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		if pos >= d.segEnd {
			return nil, io.EOF
		}
		id, size, hdrLen, err := ebml.ReadElementHeader(d.r)
		if err != nil {
			return nil, fmt.Errorf("mkv: 解析簇失败(偏移 %d): %w", pos, err)
		}
		dataSize := size
		if size == ebml.UnknownSize {
			dataSize = d.segEnd - pos - hdrLen
		}
		if id != IDCluster {
			// 跳过 SeekHead/Info/Tracks/Cues/Void 等非簇元素。
			if _, err := d.r.Seek(dataSize, io.SeekCurrent); err != nil {
				return nil, err
			}
			continue
		}
		payload, err := readPayload(d.r, dataSize)
		if err != nil {
			return nil, fmt.Errorf("mkv: 读取簇失败(偏移 %d): %w", pos, err)
		}
		cluster, err := parseCluster(payload)
		if err != nil {
			return nil, err
		}
		return cluster, nil
	}
}

func parseCluster(payload []byte) (*Cluster, error) {
	cl := &Cluster{}
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch id {
		case IDClusterTime:
			cl.Timestamp = uintOf(p)
		case IDSimpleBlock:
			frames, err := parseBlock(p, cl.Timestamp, true, true)
			if err != nil {
				return nil, err
			}
			cl.Frames = append(cl.Frames, frames...)
		case IDBlockGroup:
			frames, err := parseBlockGroup(p, cl.Timestamp)
			if err != nil {
				return nil, err
			}
			cl.Frames = append(cl.Frames, frames...)
		}
	}
	return cl, nil
}

func parseBlockGroup(payload []byte, clusterTime uint64) ([]Frame, error) {
	c := &cursor{data: payload}
	var block []byte
	hasReference := false
	for {
		id, p, err := c.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch id {
		case IDBlock:
			block = p
		case 0xFB: // ReferenceBlock:存在即非关键帧
			hasReference = true
		}
	}
	if block == nil {
		return nil, fmt.Errorf("mkv: BlockGroup 缺少 Block,容器与码流无法同步")
	}
	return parseBlock(block, clusterTime, false, !hasReference)
}

// parseBlock 解析 Block/SimpleBlock 负载,返回解 lacing 后的帧列表。
func parseBlock(payload []byte, clusterTime uint64, simple bool, defaultKey bool) ([]Frame, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("mkv: Block 长度过小,容器与码流无法同步")
	}
	trackNum, n, err := ebml.ReadVINT(bytes.NewReader(payload), false)
	if err != nil {
		return nil, fmt.Errorf("mkv: Block 轨道号损坏: %w", err)
	}
	rest := payload[n:]
	if len(rest) < 3 {
		return nil, fmt.Errorf("mkv: Block 头不完整")
	}
	relTS := int64(int16(binary.BigEndian.Uint16(rest[:2])))
	flags := rest[2]
	data := rest[3:]

	keyframe := defaultKey
	if simple {
		keyframe = flags&0x80 != 0
	}
	lacing := (flags >> 1) & 0x03

	sizes, frameData, err := splitLacing(lacing, data)
	if err != nil {
		return nil, err
	}
	frames := make([]Frame, 0, len(sizes))
	off := 0
	for _, sz := range sizes {
		if off+sz > len(frameData) {
			return nil, fmt.Errorf("mkv: lacing 帧长越界,容器与码流无法同步")
		}
		frames = append(frames, Frame{
			TrackNumber: trackNum,
			Timestamp:   int64(clusterTime) + relTS,
			Keyframe:    keyframe,
			Payload:     frameData[off : off+sz],
		})
		off += sz
	}
	return frames, nil
}

// splitLacing 按 lacing 类型计算每帧长度,返回长度列表与帧数据区。
func splitLacing(lacing byte, data []byte) ([]int, []byte, error) {
	switch lacing {
	case 0:
		return []int{len(data)}, data, nil
	case 1: // Xiph
		return splitXiph(data)
	case 2: // 定长
		return splitFixed(data)
	case 3: // EBML
		return splitEBML(data)
	}
	return nil, nil, fmt.Errorf("mkv: 未知 lacing 类型 %d", lacing)
}

func splitXiph(data []byte) ([]int, []byte, error) {
	if len(data) < 1 {
		return nil, nil, fmt.Errorf("mkv: Xiph lacing 头缺失")
	}
	count := int(data[0]) + 1
	off := 1
	sizes := make([]int, 0, count)
	total := 0
	for i := 0; i < count-1; i++ {
		sz := 0
		for {
			if off >= len(data) {
				return nil, nil, fmt.Errorf("mkv: Xiph lacing 长度表越界")
			}
			sz += int(data[off])
			off++
			if data[off-1] != 0xFF {
				break
			}
		}
		sizes = append(sizes, sz)
		total += sz
	}
	frameData := data[off:]
	last := len(frameData) - total
	if last < 0 {
		return nil, nil, fmt.Errorf("mkv: Xiph lacing 长度不一致")
	}
	return append(sizes, last), frameData, nil
}

func splitFixed(data []byte) ([]int, []byte, error) {
	if len(data) < 1 {
		return nil, nil, fmt.Errorf("mkv: 定长 lacing 头缺失")
	}
	count := int(data[0]) + 1
	frameData := data[1:]
	if count <= 0 || len(frameData)%count != 0 {
		return nil, nil, fmt.Errorf("mkv: 定长 lacing 长度不一致")
	}
	sz := len(frameData) / count
	sizes := make([]int, count)
	for i := range sizes {
		sizes[i] = sz
	}
	return sizes, frameData, nil
}

func splitEBML(data []byte) ([]int, []byte, error) {
	if len(data) < 1 {
		return nil, nil, fmt.Errorf("mkv: EBML lacing 头缺失")
	}
	count := int(data[0]) + 1
	off := 1
	sizes := make([]int, 0, count)
	first, n, err := ebml.ReadVINT(bytes.NewReader(data[off:]), false)
	if err != nil {
		return nil, nil, fmt.Errorf("mkv: EBML lacing 首帧长度损坏: %w", err)
	}
	off += n
	prev := int(first)
	sizes = append(sizes, prev)
	for i := 1; i < count-1; i++ {
		diff, m, err := readSignedVINT(data[off:])
		if err != nil {
			return nil, nil, err
		}
		off += m
		prev += diff
		if prev < 0 {
			return nil, nil, fmt.Errorf("mkv: EBML lacing 帧长为负")
		}
		sizes = append(sizes, prev)
	}
	total := 0
	for _, sz := range sizes {
		total += sz
	}
	frameData := data[off:]
	last := len(frameData) - total
	if last < 0 {
		return nil, nil, fmt.Errorf("mkv: EBML lacing 长度不一致")
	}
	return append(sizes, last), frameData, nil
}

// readSignedVINT 读取 EBML lacing 使用的有符号变长整数。
func readSignedVINT(data []byte) (int, int, error) {
	value, n, err := ebml.ReadVINT(bytes.NewReader(data), false)
	if err != nil {
		return 0, 0, fmt.Errorf("mkv: EBML lacing 差值损坏: %w", err)
	}
	bias := (int64(1) << (7*uint(n) - 1)) - 1
	return int(int64(value) - bias), n, nil
}
