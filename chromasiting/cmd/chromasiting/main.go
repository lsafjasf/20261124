// Command chromasiting demonstrates how MPEG-2 / JPEG / Center chroma
// siting declarations move a color edge across 4:2:0 subsampling and
// upsampling, and how conflicting container fields are escalated to manual
// review instead of being resolved by majority vote.
package main

import (
	"fmt"

	"chromasiting"
)

func main() {
	printSitings()
	samples := []chromasiting.Sample{
		chromasiting.RedBlueBoundary(16, 8),
		chromasiting.OddSize(13, 7),
		chromasiting.HalfPixelOffset(16, 8),
	}
	for _, smp := range samples {
		printSample(smp)
	}
	printConflicts()
}

func printSitings() {
	fmt.Println("== 支持的 chroma siting (4:2:0, 偏移为 2x2 亮度块内坐标) ==")
	for _, s := range []chromasiting.Siting{
		chromasiting.SitingMPEG2, chromasiting.SitingJPEG, chromasiting.SitingCenter,
	} {
		ox, oy := s.Offset()
		fmt.Printf("  %-14s offset=(%.1f, %.1f)\n", s, ox, oy)
	}
	fmt.Println("  注: JPEG 与 Center 物理位置相同 (0.5, 0.5)，仅来源标签不同。")
	fmt.Println()
}

func printSample(smp chromasiting.Sample) {
	fmt.Printf("== 样本 %s (%dx%d, 真实边界 x=%.2f) ==\n",
		smp.Name, smp.Img.Y.W, smp.Img.Y.H, smp.BoundaryX)
	fmt.Println("  siting          上采样前x  上采样后x  与真实差")
	sitings := []chromasiting.Siting{
		chromasiting.SitingMPEG2, chromasiting.SitingJPEG, chromasiting.SitingCenter,
	}
	for _, s := range sitings {
		r := chromasiting.MeasureBoundary(smp, s)
		fmt.Printf("  %-14s %8.2f  %8.2f  %+7.2f\n", s, r.Before, r.After, r.DeltaTrue)
	}
	fmt.Println("  错位假设 (下采样用真实 siting, 上采样用错误 siting):")
	for _, true := range sitings {
		for _, assumed := range sitings {
			if true.SamePosition(assumed) {
				continue
			}
			got := chromasiting.MeasureMismatch(smp, true, assumed)
			fmt.Printf("    真实 %-14s 假设 %-14s -> 边界 x=%.2f (偏移 %+.2f)\n",
				true, assumed, got, got-smp.BoundaryX)
		}
	}
	fmt.Println()
}

func printConflicts() {
	fmt.Println("== 容器字段冲突判定 (冲突只列人工判定, 不自动选最常见值) ==")
	i0, i1 := 0, 1
	mkvLeft, mkvHalf, mkvUnspec := uint64(1), uint64(2), uint64(0)
	av1Vertical := 1
	scenarios := []struct {
		name   string
		fields chromasiting.ContainerFields
	}{
		{"全部一致 (H.264 VUI=left, MKV=left/half, codec=mpeg2video)",
			chromasiting.ContainerFields{
				H264ChromaSampleLocationType: &i0,
				MKVChromaSitingHorz:          &mkvLeft,
				MKVChromaSitingVert:          &mkvHalf,
				CodecID:                      "mpeg2video",
			}},
		{"标签等价不算冲突 (VUI=center, codec=mjpeg)",
			chromasiting.ContainerFields{
				H264ChromaSampleLocationType: &i1,
				CodecID:                      "mjpeg",
			}},
		{"直接冲突 (VUI=left, codec=mjpeg)",
			chromasiting.ContainerFields{
				H264ChromaSampleLocationType: &i0,
				CodecID:                      "mjpeg",
			}},
		{"多数派也不自动采纳 (VUI=left, MKV=left/half, AV1=vertical, codec=mjpeg)",
			chromasiting.ContainerFields{
				H264ChromaSampleLocationType: &i0,
				MKVChromaSitingHorz:          &mkvLeft,
				MKVChromaSitingVert:          &mkvHalf,
				AV1ChromaSamplePosition:      &av1Vertical,
				CodecID:                      "mjpeg",
			}},
		{"字段缺失 (仅 MKV 未指定)",
			chromasiting.ContainerFields{
				MKVChromaSitingHorz: &mkvUnspec,
				MKVChromaSitingVert: &mkvUnspec,
			}},
	}
	for _, sc := range scenarios {
		res := chromasiting.Identify(sc.fields)
		fmt.Printf("  [%s] %s\n", res.Status, sc.name)
		if res.Status == chromasiting.StatusResolved {
			fmt.Printf("    -> 判定: %s\n", res.Siting)
		}
		for _, c := range res.Claims {
			fmt.Printf("    - %-36s %s => %s\n", c.Source, c.Detail, c.Siting)
		}
		if res.Status == chromasiting.StatusManualReview {
			fmt.Println("    -> 需人工判定, 不做多数派自动选择")
		}
	}
}
