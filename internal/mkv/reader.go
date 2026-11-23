package mkv

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"hdr10remux/internal/ebml"
)

// Track 描述一条轨道的容器级属性。
type Track struct {
	Number       uint64
	UID          uint64
	Type         uint64
	CodecID      string
	CodecPrivate []byte
	Name         string
	Language     string
	FlagDefault  bool

	// 视频
	PixelWidth  uint64
	PixelHeight uint64
	Colour      *Colour

	// 音频
	SamplingFreq float64
	Channels     uint64
	BitDepth     uint64
	HasBitDepth  bool
}

// SourceInfo 是源文件的头信息(第一遍扫描的结果)。
type SourceInfo struct {
	TimestampScale uint64
	Duration       float64
	HasDuration    bool
	Tracks         []*Track
	// SegmentDataOffset 是 Segment 数据区的文件偏移。
	SegmentDataOffset int64
	// SegmentEnd 是 Segment 数据区的结束偏移(未知长度时为文件大小)。
	SegmentEnd int64
}

// VideoTrack 返回第一条视频轨道,没有则返回 nil。
func (s *SourceInfo) VideoTrack() *Track {
	for _, t := range s.Tracks {
		if t.Type == TrackTypeVideo {
			return t
		}
	}
	return nil
}

// TrackByNumber 按轨道号查找。
func (s *SourceInfo) TrackByNumber(num uint64) *Track {
	for _, t := range s.Tracks {
		if t.Number == num {
			return t
		}
	}
	return nil
}

// maxMasterPayload 限制一次性读入内存的主元素大小(头信息理应很小)。
const maxMasterPayload = 64 << 20

func readPayload(r io.Reader, size int64) ([]byte, error) {
	if size == ebml.UnknownSize {
		return nil, fmt.Errorf("mkv: 头信息元素不允许未知长度")
	}
	if size < 0 || size > maxMasterPayload {
		return nil, fmt.Errorf("mkv: 元素负载异常 (%d 字节)", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// cursor 在内存中的主元素负载上迭代子元素。
type cursor struct {
	data []byte
	off  int
}

func (c *cursor) next() (id uint32, payload []byte, err error) {
	if c.off >= len(c.data) {
		return 0, nil, io.EOF
	}
	id64, idLen, err := ebml.ReadVINT(bytes.NewReader(c.data[c.off:]), true)
	if err != nil {
		return 0, nil, err
	}
	size64, sizeLen, err := ebml.ReadVINT(bytes.NewReader(c.data[c.off+idLen:]), false)
	if err != nil {
		return 0, nil, err
	}
	start := c.off + idLen + sizeLen
	end := len(c.data)
	if size64 != (uint64(1)<<(7*uint(sizeLen)))-1 {
		end = start + int(size64)
		if end > len(c.data) {
			return 0, nil, fmt.Errorf("mkv: 元素 0x%X 长度越界", id64)
		}
	}
	c.off = end
	return uint32(id64), c.data[start:end], nil
}

func uintOf(payload []byte) uint64 {
	var v uint64
	for _, b := range payload {
		v = v<<8 | uint64(b)
	}
	return v
}

func floatOf(payload []byte) (float64, error) {
	switch len(payload) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(payload))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(payload)), nil
	default:
		return 0, fmt.Errorf("mkv: 非法浮点长度 %d", len(payload))
	}
}

// ReadSourceInfo 扫描文件头,解析 Info 与 Tracks。
// 遇到第一个 Cluster 时停止(头信息按规范位于其前)。
func ReadSourceInfo(r io.ReadSeeker) (*SourceInfo, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	id, size, _, err := ebml.ReadElementHeader(r)
	if err != nil {
		return nil, fmt.Errorf("mkv: 读取 EBML 头失败: %w", err)
	}
	if id != IDEBMLHeader {
		return nil, fmt.Errorf("mkv: 不是 EBML/Matroska 文件")
	}
	ebmlPayload, err := readPayload(r, size)
	if err != nil {
		return nil, fmt.Errorf("mkv: 读取 EBML 头失败: %w", err)
	}
	if err := checkDocType(ebmlPayload); err != nil {
		return nil, err
	}

	id, size, _, err = ebml.ReadElementHeader(r)
	if err != nil {
		return nil, fmt.Errorf("mkv: 缺少 Segment: %w", err)
	}
	if id != IDSegment {
		return nil, fmt.Errorf("mkv: 缺少 Segment 元素")
	}
	segStart, _ := r.Seek(0, io.SeekCurrent)
	segEnd := segStart + size
	if size == ebml.UnknownSize {
		segEnd, err = r.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		if _, err := r.Seek(segStart, io.SeekStart); err != nil {
			return nil, err
		}
	}

	info := &SourceInfo{
		TimestampScale:    1000000,
		SegmentDataOffset: segStart,
		SegmentEnd:        segEnd,
	}
	for {
		pos, _ := r.Seek(0, io.SeekCurrent)
		if pos >= segEnd {
			break
		}
		id, size, hdrLen, err := ebml.ReadElementHeader(r)
		if err != nil {
			return nil, fmt.Errorf("mkv: 解析 Segment 子元素失败: %w", err)
		}
		dataSize := size
		if size == ebml.UnknownSize {
			dataSize = segEnd - pos - hdrLen
		}
		switch id {
		case IDInfo:
			payload, err := readPayload(r, dataSize)
			if err != nil {
				return nil, err
			}
			if err := parseInfo(payload, info); err != nil {
				return nil, err
			}
		case IDTracks:
			payload, err := readPayload(r, dataSize)
			if err != nil {
				return nil, err
			}
			if err := parseTracks(payload, info); err != nil {
				return nil, err
			}
		case IDCluster:
			// 头信息扫描到此为止。
			if len(info.Tracks) == 0 {
				return nil, fmt.Errorf("mkv: 缺少 Tracks,容器与码流无法同步")
			}
			return info, nil
		default:
			if _, err := r.Seek(dataSize, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}
	if len(info.Tracks) == 0 {
		return nil, fmt.Errorf("mkv: 缺少 Tracks,容器与码流无法同步")
	}
	return info, nil
}

func checkDocType(payload []byte) error {
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if id == IDEBMLDocType && string(p) != "matroska" && string(p) != "webm" {
			return fmt.Errorf("mkv: 不支持的 DocType %q", string(p))
		}
	}
	return nil
}

func parseInfo(payload []byte, info *SourceInfo) error {
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch id {
		case IDTimestampScale:
			info.TimestampScale = uintOf(p)
		case IDDuration:
			f, err := floatOf(p)
			if err != nil {
				return err
			}
			info.Duration = f
			info.HasDuration = true
		}
	}
}
