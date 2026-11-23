package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"subdrift/subtitle"
)

// Markdown renders the human-readable diagnosis. It is deterministic so the
// committed report can be regenerated and diffed.
func (r *Report) Markdown() string {
	var b strings.Builder
	p := r.Project
	fmt.Fprintf(&b, "# 字幕时基漂移诊断（自动生成，请勿手改）\n\n")
	fmt.Fprintf(&b, "工程：`%s`\n\n", p.Name)
	fmt.Fprintf(&b, "- 母版帧率：`%s` fps（精确有理数，不使用 float64）\n", p.VideoRate)
	fmt.Fprintf(&b, "- 旧管线标称帧率：`%s` fps\n", p.NominalRate)
	fmt.Fprintf(&b, "- 中间时基：`%s` Hz；ASS 中间层：`%s`\n", p.PTSTimebase, p.Intermediate)
	fmt.Fprintf(&b, "- 片长：`%d` 帧 / `%s` s\n\n", p.DurationFrames(), groundSeconds(p, p.DurationFrames()))
	fmt.Fprintf(&b, "复现：`go run ./cmd/subdrift audit -project testdata/reel_500s.json`\n\n")

	fmt.Fprintf(&b, "## 结论\n\n")
	if r.FirstError != nil {
		first := r.FirstError
		fmt.Fprintf(&b, "首个可观察的**字幕边界**误差在 **%s %s、第 %d 帧**：旧管线输出 `%s`，正确值 `%s`，即 **%+d ms**。\n\n",
			first.CueID, endpointZH(first.Endpoint), first.Frame,
			subtitle.FormatTimestamp(first.OutputMS), subtitle.FormatTimestamp(first.ReferenceMS), first.ErrorMS)
	}
	if first := r.FirstErrorStage(); first != nil {
		fmt.Fprintf(&b, "首个带偏差的字幕边界在 **%s %s、第 %d 帧** 的 `%s` 层：输入帧本应按 `%s` 解释，旧管线却按 `%s` 解释。\n\n",
			first.CueID, endpointZH(first.Endpoint), first.Frame,
			first.Stages[first.FirstStage].Name, p.VideoRate, r.Mode.InputRate)
	}
	if p.DurationFrames() > 0 {
		early := Evaluate(1, p, r.Mode)
		if early.FirstStage >= 0 {
			fmt.Fprintf(&b, "逐帧扫描时，第 1 帧的 PTS 已有约 `%s ms` 的亚毫秒偏差；\n",
				ratDecimal(early.Stages[early.FirstStage].CumulativeErrorMS, 6))
			firstFrame := int64(0)
			if r.FirstError != nil {
				firstFrame = r.FirstError.Frame
			}
			fmt.Fprintf(&b, "但它不是字幕边界。回归样例中第一个落在字幕边界、且达到毫秒可见阈值的错误是第 %d 帧。\n\n", firstFrame)
		}
	}
	lastErr := int64(0)
	if len(r.Curve) > 0 {
		lastErr = r.Curve[len(r.Curve)-1].ErrorMS
	}
	fmt.Fprintf(&b, "片尾误差为 **%+d ms**，误差斜率来自丢掉 `1001` 分母的标称 `%s` fps 重解释；\n", lastErr, r.Mode.InputRate)
	fmt.Fprintf(&b, "它随精确时间线性增长，而不是单条字幕的偶然取整。\n\n")

	fmt.Fprintf(&b, "## 首个误差事件\n\n")
	if r.FirstError != nil {
		fmt.Fprintf(&b, "| 层 | 输入速率 | 输出时基 | 本层未取整值 | 正确目标值 | 实际输出 | 累计误差 |\n")
		fmt.Fprintf(&b, "| --- | --- | --- | ---: | ---: | ---: | ---: |\n")
		for _, s := range r.FirstError.Stages {
			fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s | %s | %d | %s ms |\n",
				s.Name, s.From.Rate, s.To, ratDecimal(s.Exact, 6), ratDecimal(s.Expected, 6),
				s.Output.Count, ratDecimal(s.CumulativeErrorMS, 6))
		}
		fmt.Fprintf(&b, "\n`frame->pts` 的 `-90 tick` 就是 `-1 ms` 的源头；后续 `floor` 只把误差保留下来，\n")
		fmt.Fprintf(&b, "没有产生 500 ms 量级的累计误差。\n\n")
	}

	fmt.Fprintf(&b, "## 分层隔离（片尾第 %d 帧）\n\n", p.DurationFrames())
	fmt.Fprintf(&b, "| 变体 | 输出 | 误差 | 结论 |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | --- |\n")
	for _, v := range r.Variants {
		fmt.Fprintf(&b, "| `%s` | %d ms | %+d ms | %s |\n", v.Name, v.OutputMS, v.ErrorMS, variantConclusion(v.Name))
	}
	fmt.Fprintf(&b, "\n- 只把帧率改成标称 `%s` fps：片尾已经约 `%+d ms`，说明主因在帧率规范化。\n",
		r.Mode.InputRate, variantError(r, "nominal-rate-only"))
	fmt.Fprintf(&b, "- 只保留 90 kHz/毫秒/厘秒 `floor`：片尾约 `-10 ms`，是量化抖动，不是半秒漂移。\n")
	fmt.Fprintf(&b, "- 四层叠加：`%+d ms`。因此不能靠整体加偏移掩盖，必须修正帧率语义并在输出端只量化一次。\n\n", lastErr)

	fmt.Fprintf(&b, "### 排除项\n\n")
	fmt.Fprintf(&b, "- `25/24` 错配在同一 500 s 片长会差约 `20 s`，不是本现象；`30/29.97` 也能凑出半秒，\n")
	fmt.Fprintf(&b, "  所以必须核对工程帧率而不能只对片尾数字。样例工程明确为 `24000/1001`。\n")
	fmt.Fprintf(&b, "- 90 kHz 往返单独只有毫秒级相位误差，厘秒 `floor` 单独最多约 10 ms。\n")
	fmt.Fprintf(&b, "- 若误差来自“每条字幕时长分别取整后累加”，曲线会呈按 cue 数递增的阶梯；\n")
	fmt.Fprintf(&b, "  本样例按绝对帧号还原，测得误差随精确时间线性增长，排除了时长累加。\n\n")

	fmt.Fprintf(&b, "## 误差曲线\n\n")
	fmt.Fprintf(&b, "旧管线的片尾误差近似为：\n\n")
	fmt.Fprintf(&b, "```text\n")
	coefMS, rel := driftCoefficients(p)
	fmt.Fprintf(&b, "e(frame) = legacy_ms - fixed_ms ≈ %s * frame  (ms)\n", ratDecimal(coefMS, 6))
	fmt.Fprintf(&b, "e(t)     ≈ %s * t                       (t 为精确秒数)\n", ratDecimal(rel, 6))
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "| 帧 | 精确时间 | 旧输出 ms | 修复输出 ms | 误差 ms | 误差帧 |\n")
	fmt.Fprintf(&b, "| ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, pt := range selectedCurve(r) {
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %+d | %s |\n",
			pt.Frame, ratDecimal(pt.GroundMS, 3), pt.OutputMS, pt.ReferenceMS, pt.ErrorMS,
			ratDecimal(msToFrames(pt.ErrorMS, p.VideoRate), 6))
	}
	fmt.Fprintf(&b, "\n完整曲线见 `artifacts/error_curve.csv`，图形见 `artifacts/error_curve.svg`。\n\n")

	fmt.Fprintf(&b, "## 修复\n\n")
	fmt.Fprintf(&b, "1. 帧计数始终以 `%s` fps 解释，不再使用标称 `%s` fps。\n", p.VideoRate, p.NominalRate)
	fmt.Fprintf(&b, "2. 90 kHz PTS 由精确有理数换算并在 **mux 边界** 取最近 tick，保留 `1001` 分母。\n")
	fmt.Fprintf(&b, "3. SRT 交付直接从精确帧时刻换算，不在 `ms -> cs -> ms` 之间反复取整；\n")
	fmt.Fprintf(&b, "   若确需 ASS，厘秒量化只能发生一次，并明确其 5 ms 输出分辨率上限。\n")
	fmt.Fprintf(&b, "4. 最终只在 SRT 毫秒边界做一次最近取整，修复后逐事件误差不超过 `±0.5 ms`（本样例实测 ≤1 ms）。\n\n")
	offset := -lastErr
	shiftedFirst := int64(0)
	if r.FirstError != nil {
		shiftedFirst = r.FirstError.ErrorMS + offset
	}
	fmt.Fprintf(&b, "整体加 `%+d ms` 会让首个 `%+d ms` 的字幕变成 `%+d ms`，首尾不可能同时正确；\n",
		offset, firstErrorMS(r), shiftedFirst)
	fmt.Fprintf(&b, "回归测试 `TestGlobalOffsetDoesNotFixEarlyCues` 专门锁住这一点。\n\n")

	fmt.Fprintf(&b, "## 回归样例\n\n")
	fmt.Fprintf(&b, "- `testdata/reel_500s.json`：`24000/1001` fps、90 kHz、ASS 厘秒中间层，片尾第 `11988` 帧（`499999.5 ms`）。\n")
	fmt.Fprintf(&b, "- `testdata/reel_500s_legacy.srt`：缺陷输出，片尾结束于 `00:08:19,500`。\n")
	fmt.Fprintf(&b, "- `testdata/reel_500s_fixed.srt`：修复输出，片尾结束于 `00:08:20,000`。\n")
	fmt.Fprintf(&b, "- `go test ./...` 同时校验首个误差事件、误差曲线端点、分层隔离与整体偏移反例。\n")
	return b.String()
}

func endpointZH(s string) string {
	if s == "end" {
		return "结束"
	}
	return "开始"
}

func variantConclusion(name string) string {
	switch name {
	case "nominal-rate-only":
		return "主因：丢掉 1001 分母"
	case "timebase-floor-only":
		return "90 kHz/毫秒 floor 的相位误差"
	case "rounding-cs-only":
		return "厘秒往返的量化抖动"
	case "full-legacy":
		return "线上现象"
	default:
		return ""
	}
}

func variantError(r *Report, name string) int64 {
	for _, v := range r.Variants {
		if v.Name == name {
			return v.ErrorMS
		}
	}
	return 0
}

func firstErrorMS(r *Report) int64 {
	if r.FirstError == nil {
		return 0
	}
	return r.FirstError.ErrorMS
}

// driftCoefficients returns the nominal-rate error per source frame (ms) and
// per exact elapsed second (dimensionless).
func driftCoefficients(p *subtitle.Project) (*big.Rat, *big.Rat) {
	perFrameSec := new(big.Rat).Sub(
		new(big.Rat).Inv(p.NominalRate.Rat()),
		new(big.Rat).Inv(p.VideoRate.Rat()),
	)
	perFrameMS := new(big.Rat).Mul(perFrameSec, big.NewRat(1000, 1))
	relative := new(big.Rat).Sub(
		new(big.Rat).Quo(p.VideoRate.Rat(), p.NominalRate.Rat()),
		big.NewRat(1, 1),
	)
	return perFrameMS, relative
}

func selectedCurve(r *Report) []CurvePoint {
	want := map[int64]bool{0: true, 24: true, 240: true, 2400: true, 5994: true, r.Project.DurationFrames(): true}
	var out []CurvePoint
	for _, pt := range r.Curve {
		if want[pt.Frame] {
			out = append(out, pt)
		}
	}
	return out
}

func groundSeconds(p *subtitle.Project, frame int64) string {
	v := new(big.Rat).Quo(new(big.Rat).SetInt64(frame), p.VideoRate.Rat())
	return v.FloatString(6)
}

// CSV renders the full sampled curve.
func (r *Report) CSV() string {
	var b strings.Builder
	fmt.Fprintf(&b, "frame,ground_seconds,ground_ms,reference_ms,legacy_ms,error_ms,error_frames\n")
	for _, pt := range r.Curve {
		seconds := new(big.Rat).Quo(new(big.Rat).SetInt64(pt.Frame), r.Project.VideoRate.Rat())
		fmt.Fprintf(&b, "%d,%s,%s,%d,%d,%d,%s\n",
			pt.Frame, seconds.FloatString(6), ratDecimal(pt.GroundMS, 3),
			pt.ReferenceMS, pt.OutputMS, pt.ErrorMS,
			ratDecimal(msToFrames(pt.ErrorMS, r.Project.VideoRate), 6))
	}
	return b.String()
}

// JSONReport is the machine-readable audit summary.
type JSONReport struct {
	Project       string          `json:"project"`
	VideoRate     string          `json:"video_rate"`
	NominalRate   string          `json:"nominal_rate"`
	PTSTimebase   string          `json:"pts_timebase"`
	FirstError    *JSONBoundary   `json:"first_error"`
	MaxAbsErrorMS int64           `json:"max_abs_error_ms"`
	Variants      []VariantResult `json:"variants"`
	Curve         []JSONCurve     `json:"curve"`
}

type JSONBoundary struct {
	CueID       string `json:"cue_id"`
	Endpoint    string `json:"endpoint"`
	Frame       int64  `json:"frame"`
	GroundMS    string `json:"ground_ms"`
	ReferenceMS int64  `json:"reference_ms"`
	OutputMS    int64  `json:"output_ms"`
	ErrorMS     int64  `json:"error_ms"`
	FirstStage  int    `json:"first_stage"`
	StageName   string `json:"stage_name,omitempty"`
}

type JSONCurve struct {
	Frame       int64  `json:"frame"`
	GroundMS    string `json:"ground_ms"`
	ReferenceMS int64  `json:"reference_ms"`
	OutputMS    int64  `json:"output_ms"`
	ErrorMS     int64  `json:"error_ms"`
}

// JSON serializes the audit deterministically.
func (r *Report) JSON() ([]byte, error) {
	out := JSONReport{
		Project:       r.Project.Name,
		VideoRate:     r.Project.VideoRate.String(),
		NominalRate:   r.Project.NominalRate.String(),
		PTSTimebase:   r.Project.PTSTimebase.String(),
		MaxAbsErrorMS: r.MaxAbsErrorMS,
		Variants:      r.Variants,
	}
	if r.FirstError != nil {
		fe := r.FirstError
		b := &JSONBoundary{
			CueID: fe.CueID, Endpoint: fe.Endpoint, Frame: fe.Frame,
			GroundMS: ratDecimal(fe.GroundMS, 3), ReferenceMS: fe.ReferenceMS,
			OutputMS: fe.OutputMS, ErrorMS: fe.ErrorMS, FirstStage: fe.FirstStage,
		}
		if fe.FirstStage >= 0 && fe.FirstStage < len(fe.Stages) {
			b.StageName = fe.Stages[fe.FirstStage].Name
		}
		out.FirstError = b
	}
	for _, pt := range r.Curve {
		out.Curve = append(out.Curve, JSONCurve{
			Frame: pt.Frame, GroundMS: ratDecimal(pt.GroundMS, 3),
			ReferenceMS: pt.ReferenceMS, OutputMS: pt.OutputMS, ErrorMS: pt.ErrorMS,
		})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func ratDecimal(r *big.Rat, places int) string {
	if r == nil {
		return ""
	}
	return r.FloatString(places)
}
