// Package remux 负责把已确认的 HDR10 元数据迁移到新封装,
// 并保证任何失败都原子回滚、成功时重新解析校验音视频载荷。
package remux

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"

	"hdr10remux/internal/mkv"
)

// 各类失败的哨兵错误,调用方可用 errors.Is 判断。
var (
	// ErrWritePermission 表示输出目录/文件写权限不足。
	ErrWritePermission = errors.New("写权限不足")
	// ErrSync 表示容器与码流无法同步(结构损坏、时间戳倒退等)。
	ErrSync = errors.New("容器与码流无法同步")
	// ErrNotConfirmed 表示 HDR10 元数据未确认,拒绝迁移。
	ErrNotConfirmed = errors.New("HDR10 元数据未确认")
	// ErrVerify 表示输出校验失败。
	ErrVerify = errors.New("输出校验失败")
)

// Options 是一次重封装任务的参数。
type Options struct {
	InputPath  string
	OutputPath string
	// Log 接收进度信息,可为 nil。
	Log func(format string, args ...any)
}

// trackDigest 记录单条轨道的载荷摘要。
type trackDigest struct {
	frames uint64
	sum    [sha256.Size]byte
}

// Run 执行重封装。成功时输出文件已落盘并通过校验;
// 任何失败都不会留下输出或临时文件,输入文件保持原样。
func Run(opts Options) (err error) {
	log := opts.Log
	if log == nil {
		log = func(string, ...any) {}
	}
	if opts.InputPath == "" || opts.OutputPath == "" {
		return fmt.Errorf("输入/输出路径不能为空")
	}
	inAbs, errAbs := filepath.Abs(opts.InputPath)
	outAbs, errAbs2 := filepath.Abs(opts.OutputPath)
	if errAbs == nil && errAbs2 == nil && inAbs == outAbs {
		return fmt.Errorf("输出路径不能与输入相同")
	}

	in, err := os.Open(opts.InputPath)
	if err != nil {
		return fmt.Errorf("打开输入失败: %w", err)
	}
	defer in.Close()

	info, err := mkv.ReadSourceInfo(in)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSync, err)
	}
	if err := validateStructure(info); err != nil {
		return err
	}
	video := info.VideoTrack()
	if video.Colour == nil {
		return fmt.Errorf("%w: 视频轨道缺少 Colour 信息", ErrNotConfirmed)
	}
	hdr, cerr := video.Colour.ConfirmHDR10()
	if cerr != nil {
		return fmt.Errorf("%w: %v", ErrNotConfirmed, cerr)
	}
	log("HDR10 元数据已确认: MaxCLL=%d MaxFALL=%d 亮度[%.4f, %.1f]nit",
		hdr.MaxCLL, hdr.MaxFALL, hdr.Mastering.LuminanceMin, hdr.Mastering.LuminanceMax)

	// 在输出目录创建临时文件;权限不足在此直接失败,不产生任何残留。
	outDir := filepath.Dir(opts.OutputPath)
	tmp, err := os.CreateTemp(outDir, ".hdr10remux-*.tmp")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w: 无法在 %s 创建临时文件", ErrWritePermission, outDir)
		}
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	srcDigests, err := streamCopy(in, info, hdr, tmp, log)
	if err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("落盘失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	if err := verifyOutput(tmpPath, hdr, srcDigests, log); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, opts.OutputPath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w: 无法写入 %s", ErrWritePermission, opts.OutputPath)
		}
		return fmt.Errorf("替换输出文件失败: %w", err)
	}
	syncDir(outDir)
	committed = true
	log("完成: %s", opts.OutputPath)
	return nil
}

// validateStructure 做头信息级别的同步校验。
func validateStructure(info *mkv.SourceInfo) error {
	seen := map[uint64]bool{}
	hasVideo := false
	for _, t := range info.Tracks {
		if seen[t.Number] {
			return fmt.Errorf("%w: 轨道号 %d 重复", ErrSync, t.Number)
		}
		seen[t.Number] = true
		if t.Type == mkv.TrackTypeVideo {
			hasVideo = true
		}
	}
	if !hasVideo {
		return fmt.Errorf("%w: 没有视频轨道", ErrSync)
	}
	return nil
}

// streamCopy 流式完成:解复用输入 -> 逐帧哈希 -> 写入新封装。
func streamCopy(in *os.File, info *mkv.SourceInfo, hdr *mkv.HDR10, out *os.File, log func(string, ...any)) (map[uint64]*trackDigest, error) {
	header := &mkv.MuxHeader{
		TimestampScale: info.TimestampScale,
		Duration:       info.Duration,
		HasDuration:    info.HasDuration,
		MuxingApp:      "hdr10-remux",
		WritingApp:     "hdr10-remux (pure Go, zero deps)",
	}
	for _, t := range info.Tracks {
		mt := mkv.MuxTrack{Source: t}
		if t.Type == mkv.TrackTypeVideo {
			mt.HDR10 = hdr
		}
		header.Tracks = append(header.Tracks, mt)
	}

	bw := bufio.NewWriterSize(out, 1<<20)
	muxer := mkv.NewMuxer(bw)
	if err := muxer.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("写入头部失败: %w", err)
	}

	if _, err := in.Seek(info.SegmentDataOffset, io.SeekStart); err != nil {
		return nil, err
	}
	demuxer := mkv.NewDemuxer(in, info)
	digests := map[uint64]*trackDigest{}
	hashers := map[uint64]hash.Hash{}
	for _, t := range info.Tracks {
		digests[t.Number] = &trackDigest{}
		hashers[t.Number] = sha256.New()
	}

	var prevClusterTS uint64
	var clusterCount, frameCount uint64
	for {
		cl, err := demuxer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSync, err)
		}
		if clusterCount > 0 && cl.Timestamp < prevClusterTS {
			return nil, fmt.Errorf("%w: 簇时间戳倒退 (%d < %d)", ErrSync, cl.Timestamp, prevClusterTS)
		}
		prevClusterTS = cl.Timestamp
		for _, f := range cl.Frames {
			d, ok := digests[f.TrackNumber]
			if !ok {
				return nil, fmt.Errorf("%w: 帧引用了不存在的轨道 %d", ErrSync, f.TrackNumber)
			}
			if f.Timestamp < 0 {
				return nil, fmt.Errorf("%w: 轨道 %d 出现负时间戳", ErrSync, f.TrackNumber)
			}
			hashers[f.TrackNumber].Write(f.Payload)
			d.frames++
			frameCount++
		}
		if err := muxer.WriteCluster(cl); err != nil {
			return nil, fmt.Errorf("写入簇失败: %w", err)
		}
		clusterCount++
	}
	if err := bw.Flush(); err != nil {
		return nil, fmt.Errorf("写入输出失败: %w", err)
	}
	for num, h := range hashers {
		sum := h.Sum(nil)
		copy(digests[num].sum[:], sum)
	}
	log("已迁移 %d 个簇 / %d 帧", clusterCount, frameCount)
	return digests, nil
}

// verifyOutput 重新解析输出文件:
// 1. HDR10 元数据仍已确认且与源一致;
// 2. 旧字段彻底消失;
// 3. 每条音视频轨道的载荷哈希与帧数与源完全一致。
func verifyOutput(path string, hdr *mkv.HDR10, src map[uint64]*trackDigest, log func(string, ...any)) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: 无法重新打开输出: %v", ErrVerify, err)
	}
	defer f.Close()

	info, err := mkv.ReadSourceInfo(f)
	if err != nil {
		return fmt.Errorf("%w: 输出无法重新解析: %v", ErrVerify, err)
	}
	video := info.VideoTrack()
	if video == nil || video.Colour == nil {
		return fmt.Errorf("%w: 输出缺少视频 Colour 信息", ErrVerify)
	}
	outHDR, err := video.Colour.ConfirmHDR10()
	if err != nil {
		return fmt.Errorf("%w: 输出 HDR10 未确认: %v", ErrVerify, err)
	}
	if *outHDR != *hdr {
		return fmt.Errorf("%w: 输出 HDR10 元数据与源不一致", ErrVerify)
	}

	// 结构化扫描,确认旧字段彻底消失。
	legacy := map[uint32]bool{}
	for _, id := range mkv.LegacyIDs() {
		legacy[id] = true
	}
	foundLegacy := []uint32{}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := mkv.WalkElements(f, func(id uint32) {
		if legacy[id] {
			foundLegacy = append(foundLegacy, id)
		}
	}); err != nil {
		return fmt.Errorf("%w: 扫描输出结构失败: %v", ErrVerify, err)
	}
	if len(foundLegacy) > 0 {
		return fmt.Errorf("%w: 输出仍包含旧字段 0x%X", ErrVerify, foundLegacy[0])
	}

	// 重新解复用并逐轨道比对载荷哈希。
	if _, err := f.Seek(info.SegmentDataOffset, io.SeekStart); err != nil {
		return err
	}
	demuxer := mkv.NewDemuxer(f, info)
	digests := map[uint64]*trackDigest{}
	hashers := map[uint64]hash.Hash{}
	for _, t := range info.Tracks {
		digests[t.Number] = &trackDigest{}
		hashers[t.Number] = sha256.New()
	}
	for {
		cl, err := demuxer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: 重新解复用输出失败: %v", ErrVerify, err)
		}
		for _, fr := range cl.Frames {
			d, ok := digests[fr.TrackNumber]
			if !ok {
				return fmt.Errorf("%w: 输出出现未知轨道 %d", ErrVerify, fr.TrackNumber)
			}
			hashers[fr.TrackNumber].Write(fr.Payload)
			d.frames++
		}
	}
	for num, h := range hashers {
		var sum [sha256.Size]byte
		copy(sum[:], h.Sum(nil))
		want, ok := src[num]
		if !ok {
			return fmt.Errorf("%w: 输出多出轨道 %d", ErrVerify, num)
		}
		got := digests[num]
		if got.frames != want.frames {
			return fmt.Errorf("%w: 轨道 %d 帧数不一致 (%d != %d)", ErrVerify, num, got.frames, want.frames)
		}
		if sum != want.sum {
			return fmt.Errorf("%w: 轨道 %d 载荷哈希不一致", ErrVerify, num)
		}
		log("轨道 %d 校验通过: %d 帧, SHA-256 %x", num, got.frames, sum[:8])
	}
	for num := range src {
		if _, ok := digests[num]; !ok {
			return fmt.Errorf("%w: 输出缺少轨道 %d", ErrVerify, num)
		}
	}
	return nil
}

// syncDir 尽力而为地 fsync 目录,保证 rename 结果落盘。
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	d.Sync()
}
