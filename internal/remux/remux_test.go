package remux

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hdr10remux/internal/mkv"
)

func quietLog(string, ...any) {}

// assertNoTemp 确认目录下没有残留的临时文件。
func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".hdr10remux-") {
			t.Fatalf("存在残留临时文件: %s", e.Name())
		}
	}
}

func TestRemuxSuccess(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	out := filepath.Join(dir, "out.mkv")
	buildTestInput(t, in, true)

	if err := Run(Options{InputPath: in, OutputPath: out, Log: quietLog}); err != nil {
		t.Fatalf("重封装失败: %v", err)
	}
	assertNoTemp(t, dir)

	// 重新解析输出:HDR10 已确认且数值正确。
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := mkv.ReadSourceInfo(f)
	if err != nil {
		t.Fatalf("输出无法解析: %v", err)
	}
	video := info.VideoTrack()
	if video == nil || video.Colour == nil {
		t.Fatal("输出缺少视频 Colour")
	}
	hdr, err := video.Colour.ConfirmHDR10()
	if err != nil {
		t.Fatalf("输出 HDR10 未确认: %v", err)
	}
	if hdr.MaxCLL != 1000 || hdr.MaxFALL != 400 {
		t.Fatalf("MaxCLL/MaxFALL 错误: %d/%d", hdr.MaxCLL, hdr.MaxFALL)
	}
	if hdr.Mastering.LuminanceMax != 1000.0 || hdr.Primaries != 9 || hdr.Transfer != 16 {
		t.Fatalf("HDR10 字段迁移错误: %+v", hdr)
	}

	// 旧字段彻底消失。
	legacy := map[uint32]bool{}
	for _, id := range mkv.LegacyIDs() {
		legacy[id] = true
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	err = mkv.WalkElements(f, func(id uint32) {
		if legacy[id] {
			t.Errorf("输出仍包含旧字段 0x%X", id)
		}
	})
	if err != nil {
		t.Fatalf("扫描输出失败: %v", err)
	}

	// 帧序列与时间戳逐帧一致。
	if _, err := f.Seek(info.SegmentDataOffset, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	demuxer := mkv.NewDemuxer(f, info)
	var got []struct {
		track uint64
		ts    int64
		data  string
	}
	for {
		cl, err := demuxer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("解复用输出失败: %v", err)
		}
		for _, fr := range cl.Frames {
			got = append(got, struct {
				track uint64
				ts    int64
				data  string
			}{fr.TrackNumber, fr.Timestamp, string(fr.Payload)})
		}
	}
	if len(got) != len(wantFrames) {
		t.Fatalf("帧数不一致: %d != %d", len(got), len(wantFrames))
	}
	for i, w := range wantFrames {
		if got[i].track != w.track || got[i].ts != w.ts || got[i].data != w.data {
			t.Errorf("帧 %d 不一致: got {%d %d %q}, want {%d %d %q}",
				i, got[i].track, got[i].ts, got[i].data, w.track, w.ts, w.data)
		}
	}
}

func TestRemuxNotConfirmed(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	out := filepath.Join(dir, "out.mkv")
	buildTestInput(t, in, false) // 缺少 MaxCLL/MaxFALL

	err := Run(Options{InputPath: in, OutputPath: out, Log: quietLog})
	if !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("期望 ErrNotConfirmed,得到: %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("失败时不应产生输出文件")
	}
	assertNoTemp(t, dir)
}

func TestRemuxCorruptInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	out := filepath.Join(dir, "out.mkv")
	buildTestInput(t, in, true)

	// 截断到第二个簇中间,制造码流损坏。
	data, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, data[:len(data)-20], 0o644); err != nil {
		t.Fatal(err)
	}

	err = Run(Options{InputPath: in, OutputPath: out, Log: quietLog})
	if !errors.Is(err, ErrSync) {
		t.Fatalf("期望 ErrSync,得到: %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("失败时不应产生输出文件")
	}
	assertNoTemp(t, dir)
}

func TestRemuxWritePermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 用户不受权限位约束,跳过")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	buildTestInput(t, in, true)

	roDir := filepath.Join(dir, "readonly")
	if err := os.Mkdir(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(roDir, 0o700)
	out := filepath.Join(roDir, "out.mkv")

	err := Run(Options{InputPath: in, OutputPath: out, Log: quietLog})
	if !errors.Is(err, ErrWritePermission) {
		t.Fatalf("期望 ErrWritePermission,得到: %v", err)
	}
	assertNoTemp(t, roDir)
}

func TestRemuxSamePath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	buildTestInput(t, in, true)
	if err := Run(Options{InputPath: in, OutputPath: in, Log: quietLog}); err == nil {
		t.Fatal("输入输出同路径应被拒绝")
	}
}
