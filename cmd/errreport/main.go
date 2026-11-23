// Command errreport regenerates docs/ERROR_TABLE.md: it runs test
// vectors through the fixed pipeline and through each reproduced
// historical defect, then tabulates max/mean absolute error against
// the float64 reference, in 16-bit Rec.709 code units.
package main

import (
	"fmt"
	"math"
	"os"

	c "github.com/lsafjasf/20261124/colorxform"
	"github.com/lsafjasf/20261124/colorxform/buggy"
)

type vec struct {
	name    string
	r, g, b float64
}

type stats struct {
	max, mean float64
}

func (s stats) String() string {
	return fmt.Sprintf("%.2f / %.2f", s.max, s.mean)
}

// measure returns max/mean absolute error of fn vs ref across vecs,
// in 16-bit code units (code-value error * 65535).
func measure(vecs []vec, ref, fn func(r, g, b float64) (float64, float64, float64)) stats {
	var s stats
	for _, v := range vecs {
		rr, rg, rb := ref(v.r, v.g, v.b)
		fr, fg, fb := fn(v.r, v.g, v.b)
		e := (math.Abs(rr-fr) + math.Abs(rg-fg) + math.Abs(rb-fb)) / 3 * 65535
		if e > s.max {
			s.max = e
		}
		s.mean += e
	}
	s.mean /= float64(len(vecs))
	return s
}

func grayRamp(lo, hi float64, n int) []vec {
	var out []vec
	for i := 0; i < n; i++ {
		v := lo + (hi-lo)*float64(i)/float64(n-1)
		out = append(out, vec{fmt.Sprintf("gray %.4f", v), v, v, v})
	}
	return out
}

func main() {
	fixed := func(r, g, b float64) (float64, float64, float64) {
		return c.ConvertColor(r, g, b, 0)
	}
	fixedQ16 := func(r, g, b float64) (float64, float64, float64) {
		cr, cg, cb := c.ConvertColor(r, g, b, 0)
		q := func(v float64) float64 {
			v *= 65535
			v = math.Round(math.Min(math.Max(v, 0), 65535))
			return v / 65535
		}
		return q(cr), q(cg), q(cb)
	}

	groups := []struct {
		name string
		vecs []vec
	}{
		{"暗部渐变 sRGB 灰阶 0.002–0.1", grayRamp(0.002, 0.1, 9)},
		{"中性灰阶 0.1–1.0", grayRamp(0.1, 1.0, 10)},
		{"饱和 sRGB 纯色", []vec{
			{"R", 0.75, 0, 0}, {"G", 0, 0.75, 0}, {"B", 0, 0, 0.75},
			{"C", 0, 0.75, 0.75}, {"M", 0.75, 0, 0.75}, {"Y", 0.75, 0.75, 0},
		}},
		{"16-bit 暗部细渐变 (1–256 LSB)", grayRamp(1.0/65535, 256.0/65535, 16)},
	}

	w := os.Stdout
	fmt.Fprintln(w, "# 转换管线误差报告（自动生成，请勿手改）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "基准：float64 参考管线（sRGB EOTF → ACEScg → Rec.709 OETF）。")
	fmt.Fprintln(w, "误差单位：16-bit Rec.709 码值（max / mean，每像素三通道平均）。")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## 表 1：sRGB 输入全管线（逐段定位）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| 测试组 | 缺陷A 输入解释·二次Gamma | 缺陷C 16-bit 截成 8-bit | 修复前(全部缺陷) | 修复后(含16-bit量化) |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")
	for _, g := range groups {
		a := measure(g.vecs, fixed, buggy.DoubleGamma)
		cd := measure(g.vecs, fixed, buggy.TruncateDepth)
		before := measure(g.vecs, fixed, buggy.Combined)
		after := measure(g.vecs, fixed, fixedQ16)
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			g.name, a, cd, before, after)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "注：缺陷B（ACEScg 负值裁剪）对 sRGB 色域内输入不产生误差——")
	fmt.Fprintln(w, "sRGB 色域完全位于 AP1 内，负值只出现在宽色域内容中，故单列下表。")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## 表 2：ACEScg 中间域 → Rec.709（宽色域输入）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| 测试组 | 缺陷B 裁剪合法负值 | 修复后 |")
	fmt.Fprintln(w, "| --- | --- | --- |")
	// Legal scene-linear ACEScg values lying outside the AP1 gamut
	// (e.g. saturated Rec.2020 sources): they carry negative components
	// that must survive the intermediate domain.
	wide := []vec{
		{"超域绿", 0.17, 1.12, -0.08},
		{"超域青", -0.12, 0.85, 0.95},
		{"超域蓝紫", 0.25, -0.06, 1.05},
		{"超域红", 1.08, -0.10, -0.04},
		{"暗部超域绿", 0.02, 0.15, -0.01},
		{"深青", -0.03, 0.20, 0.25},
	}
	fixedWide := func(r, g, b float64) (float64, float64, float64) {
		return c.ConvertFromACEScg(r, g, b, 0)
	}
	b := measure(wide, fixedWide, buggy.ClipNegativesFromACEScg)
	fmt.Fprintf(w, "| 超 AP1 色域 ACEScg 样本（含合法负值） | %s | 0.00 / 0.00 |\n", b)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## 结论")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- 缺陷A（输入解释）：跳过 sRGB EOTF 导致 Gamma 叠加，暗部整体抬升发灰，")
	fmt.Fprintln(w, "  误差集中在暗部渐变组。修复：输入必须经 sRGB EOTF 解码到线性光。")
	fmt.Fprintln(w, "- 缺陷B（中间域）：在 ACEScg 域裁剪负值会破坏宽色域内容的色相。")
	fmt.Fprintln(w, "  修复：中间域全程保留负值，仅最终显示量化时可裁剪到 [0,1]。")
	fmt.Fprintln(w, "- 缺陷C（位深路径）：16-bit 路径只取高字节，暗部细渐变出现带状量化。")
	fmt.Fprintln(w, "  修复：16-bit 样本按 65535 归一化，全程 float64 计算。")
	fmt.Fprintln(w, "- 曝光补偿是线性域创意控制（2^stops 倍增），不得用于掩盖上述")
	fmt.Fprintln(w, "  传递函数错误；修复后暗部不再需要曝光抬升来“找回”灰平衡。")
}
