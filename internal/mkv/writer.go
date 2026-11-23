package mkv

import (
	"encoding/binary"
	"fmt"
	"io"

	"hdr10remux/internal/ebml"
)

// MuxTrack 是写入侧的一条轨道:源轨道属性 + 已确认的 HDR10(仅视频)。
type MuxTrack struct {
	Source *Track
	HDR10  *HDR10 // 仅视频轨道非空
}

// MuxHeader 描述输出文件的头信息。
type MuxHeader struct {
	TimestampScale uint64
	Duration       float64
	HasDuration    bool
	Tracks         []MuxTrack
	MuxingApp      string
	WritingApp     string
}

// Muxer 流式写出一个新的 Matroska 文件。
// Segment 使用未知长度,簇逐个落盘,避免整文件驻留内存。
type Muxer struct {
	w io.Writer
}

// NewMuxer 创建封装器。
func NewMuxer(w io.Writer) *Muxer {
	return &Muxer{w: w}
}

// WriteHeader 写入 EBML 头、Segment 起始、Info 与 Tracks。
func (m *Muxer) WriteHeader(h *MuxHeader) error {
	var head []byte
	head, finish := ebml.AppendMaster(head, IDEBMLHeader)
	head = ebml.AppendUint(head, IDEBMLVersion, 1)
	head = ebml.AppendUint(head, IDEBMLReadVersion, 1)
	head = ebml.AppendUint(head, IDEBMLMaxIDLength, 4)
	head = ebml.AppendUint(head, IDEBMLMaxSizeLength, 8)
	head = ebml.AppendString(head, IDEBMLDocType, "matroska")
	head = ebml.AppendUint(head, IDEBMLDocTypeVer, 4)
	head = ebml.AppendUint(head, IDEBMLDocTypeRead, 2)
	head = finish(head)
	if _, err := m.w.Write(head); err != nil {
		return err
	}

	var seg []byte
	seg = ebml.AppendID(seg, IDSegment)
	seg = ebml.AppendUnknownSize(seg)
	if _, err := m.w.Write(seg); err != nil {
		return err
	}

	var info []byte
	info, infoDone := ebml.AppendMaster(info, IDInfo)
	info = ebml.AppendUint(info, IDTimestampScale, h.TimestampScale)
	if h.HasDuration {
		info = ebml.AppendFloat64(info, IDDuration, h.Duration)
	}
	info = ebml.AppendString(info, IDMuxingApp, h.MuxingApp)
	info = ebml.AppendString(info, IDWritingApp, h.WritingApp)
	info = infoDone(info)
	if _, err := m.w.Write(info); err != nil {
		return err
	}

	var tracks []byte
	tracks, tracksDone := ebml.AppendMaster(tracks, IDTracks)
	for _, mt := range h.Tracks {
		tracks = appendTrackEntry(tracks, mt)
	}
	tracks = tracksDone(tracks)
	if _, err := m.w.Write(tracks); err != nil {
		return err
	}
	return nil
}

func appendTrackEntry(dst []byte, mt MuxTrack) []byte {
	src := mt.Source
	dst, done := ebml.AppendMaster(dst, IDTrackEntry)
	dst = ebml.AppendUint(dst, IDTrackNumber, src.Number)
	dst = ebml.AppendUint(dst, IDTrackUID, src.UID)
	dst = ebml.AppendUint(dst, IDTrackType, src.Type)
	dst = ebml.AppendUint(dst, IDFlagEnabled, 1)
	if src.FlagDefault {
		dst = ebml.AppendUint(dst, IDFlagDefault, 1)
	}
	// FlagLacing=0:输出一律不使用 lacing。
	dst = ebml.AppendUint(dst, IDFlagLacing, 0)
	dst = ebml.AppendString(dst, IDCodecID, src.CodecID)
	if len(src.CodecPrivate) > 0 {
		dst = ebml.AppendBytes(dst, IDCodecPrivate, src.CodecPrivate)
	}
	if src.Language != "" {
		dst = ebml.AppendString(dst, IDTrackLanguage, src.Language)
	}
	if src.Name != "" {
		dst = ebml.AppendString(dst, IDTrackName, src.Name)
	}
	if src.Type == TrackTypeVideo {
		dst = appendVideo(dst, src, mt.HDR10)
	} else if src.Type == TrackTypeAudio {
		dst = appendAudio(dst, src)
	}
	return done(dst)
}

func appendVideo(dst []byte, src *Track, hdr *HDR10) []byte {
	dst, done := ebml.AppendMaster(dst, IDVideo)
	dst = ebml.AppendUint(dst, IDPixelWidth, src.PixelWidth)
	dst = ebml.AppendUint(dst, IDPixelHeight, src.PixelHeight)
	if hdr != nil {
		dst = appendHDR10Colour(dst, hdr)
	}
	return done(dst)
}

// appendHDR10Colour 只写入迁移后的 HDR10 字段;旧字段(ColourSpace、Gamma 等)
// 不会出现在输出中。
func appendHDR10Colour(dst []byte, hdr *HDR10) []byte {
	dst, done := ebml.AppendMaster(dst, IDColour)
	dst = ebml.AppendUint(dst, IDMatrixCoefficients, hdr.Matrix)
	if hdr.HasRange {
		dst = ebml.AppendUint(dst, IDRange, hdr.Range)
	}
	dst = ebml.AppendUint(dst, IDTransferCharac, hdr.Transfer)
	dst = ebml.AppendUint(dst, IDPrimaries, hdr.Primaries)
	dst = ebml.AppendUint(dst, IDMaxCLL, hdr.MaxCLL)
	dst = ebml.AppendUint(dst, IDMaxFALL, hdr.MaxFALL)
	dst, masterDone := ebml.AppendMaster(dst, IDMasteringMeta)
	m := hdr.Mastering
	dst = ebml.AppendFloat64(dst, IDPrimaryRX, m.PrimaryRX)
	dst = ebml.AppendFloat64(dst, IDPrimaryRY, m.PrimaryRY)
	dst = ebml.AppendFloat64(dst, IDPrimaryGX, m.PrimaryGX)
	dst = ebml.AppendFloat64(dst, IDPrimaryGY, m.PrimaryGY)
	dst = ebml.AppendFloat64(dst, IDPrimaryBX, m.PrimaryBX)
	dst = ebml.AppendFloat64(dst, IDPrimaryBY, m.PrimaryBY)
	dst = ebml.AppendFloat64(dst, IDWhitePointX, m.WhitePointX)
	dst = ebml.AppendFloat64(dst, IDWhitePointY, m.WhitePointY)
	dst = ebml.AppendFloat64(dst, IDLuminanceMax, m.LuminanceMax)
	dst = ebml.AppendFloat64(dst, IDLuminanceMin, m.LuminanceMin)
	dst = masterDone(dst)
	return done(dst)
}

func appendAudio(dst []byte, src *Track) []byte {
	dst, done := ebml.AppendMaster(dst, IDAudio)
	if src.SamplingFreq > 0 {
		dst = ebml.AppendFloat64(dst, IDSamplingFreq, src.SamplingFreq)
	}
	if src.Channels > 0 {
		dst = ebml.AppendUint(dst, IDChannels, src.Channels)
	}
	if src.HasBitDepth {
		dst = ebml.AppendUint(dst, IDBitDepth, src.BitDepth)
	}
	return done(dst)
}

// WriteCluster 写出一个簇;所有帧以无 lacing 的 SimpleBlock 表示。
// 帧时间戳会先换算为簇相对值并校验 int16 范围。
func (m *Muxer) WriteCluster(cl *Cluster) error {
	var buf []byte
	buf, done := ebml.AppendMaster(buf, IDCluster)
	buf = ebml.AppendUint(buf, IDClusterTime, cl.Timestamp)
	for _, f := range cl.Frames {
		rel := f.Timestamp - int64(cl.Timestamp)
		if rel < -32768 || rel > 32767 {
			return fmt.Errorf("mkv: 帧时间戳超出簇范围(轨道 %d),容器与码流无法同步", f.TrackNumber)
		}
		buf = appendSimpleBlock(buf, f, rel)
	}
	buf = done(buf)
	_, err := m.w.Write(buf)
	return err
}

// appendSimpleBlock 写出单个 SimpleBlock:轨道号 VINT + int16 相对时间戳 + 标志 + 载荷。
func appendSimpleBlock(dst []byte, f Frame, relTS int64) []byte {
	var block []byte
	block = ebml.AppendSize(block, f.TrackNumber) // 轨道号按 VINT 编码
	var tsb [2]byte
	binary.BigEndian.PutUint16(tsb[:], uint16(int16(relTS)))
	block = append(block, tsb[:]...)
	var flags byte
	if f.Keyframe {
		flags |= 0x80
	}
	block = append(block, flags)
	block = append(block, f.Payload...)
	dst = ebml.AppendHeader(dst, IDSimpleBlock, uint64(len(block)))
	return append(dst, block...)
}
