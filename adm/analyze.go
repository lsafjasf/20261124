package adm

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ObjectAnalysis summarises one object across the whole chain.
type ObjectAnalysis struct {
	Name          string
	AuthoredMaxAz float64 // max |azimuth| over authored blocks
	AuthoredMaxEl float64
	FinalEnergy   []float64 // aggregated per layout speaker
	TotalEnergy   float64
	CenterRatio   float64 // share of energy in C (or balanced L/R)
	SpreadDeg     float64 // power-weighted angular RMS of final distribution
	StageStatus   map[string]string
	CollapseStage string // first stage where the object folds, "" = none
	Unrenderable  bool
	Suggestion    string
}

// Analyze computes per-object stage metrics and collapse localisation.
func Analyze(res *RenderResult) []*ObjectAnalysis {
	byName := map[string]*ObjectAnalysis{}
	order := []string{}
	for _, pb := range res.Blocks {
		oa, ok := byName[pb.Object]
		if !ok {
			oa = &ObjectAnalysis{Name: pb.Object, StageStatus: map[string]string{
				StageSource: "OK", StageNormalized: "OK", StagePanned: "OK",
			}}
			if res.Downmix {
				oa.StageStatus[StageDownmix] = "OK"
			}
			oa.FinalEnergy = make([]float64, len(res.Layout.Speakers))
			byName[pb.Object] = oa
			order = append(order, pb.Object)
		}
		auth := pb.Authored
		if auth.Cartesian {
			auth = cartesianToPolar(auth)
		}
		oa.AuthoredMaxAz = math.Max(oa.AuthoredMaxAz, math.Abs(auth.Azimuth))
		oa.AuthoredMaxEl = math.Max(oa.AuthoredMaxEl, math.Abs(auth.Elevation))
		for i, e := range pb.Energy {
			oa.FinalEnergy[i] += e
			oa.TotalEnergy += e
		}
		for _, ev := range pb.Events {
			oa.StageStatus[ev.Stage] = ev.Detail
		}
	}
	for _, ev := range res.Events {
		if _, ok := byName[ev.Object]; !ok {
			oa := &ObjectAnalysis{Name: ev.Object, StageStatus: map[string]string{StageSource: ev.Detail}}
			oa.FinalEnergy = make([]float64, len(res.Layout.Speakers))
			byName[ev.Object] = oa
			order = append(order, ev.Object)
		} else {
			byName[ev.Object].StageStatus[ev.Stage] = ev.Detail
		}
	}
	out := []*ObjectAnalysis{}
	for _, name := range order {
		oa := byName[name]
		finalize(oa, res)
		out = append(out, oa)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func finalize(oa *ObjectAnalysis, res *RenderResult) {
	if oa.TotalEnergy > 0 {
		oa.CenterRatio = centerRatio(oa.FinalEnergy, res.Layout)
		oa.SpreadDeg = spreadDeg(oa.FinalEnergy, res.Layout)
	}
	stages := []string{StageSource, StageNormalized, StagePanned}
	if res.Downmix {
		stages = append(stages, StageDownmix)
	}
	for _, st := range stages {
		if oa.StageStatus[st] != "" && oa.StageStatus[st] != "OK" {
			oa.CollapseStage = st
			break
		}
	}
	authored := math.Hypot(oa.AuthoredMaxAz, oa.AuthoredMaxEl)
	collapsed := oa.TotalEnergy > 0 && oa.CenterRatio > 0.8 && oa.SpreadDeg < 10
	switch {
	case oa.StageStatus[StageSource] != "OK" && oa.StageStatus[StageSource] != "":
		oa.Unrenderable = true
		oa.Suggestion = "ADM 结构不完整：人工核对 audioObject→audioPackFormat→audioChannelFormat 引用链，必要时重新导出"
	case oa.TotalEnergy == 0 && oa.CollapseStage != "":
		oa.Unrenderable = true
		oa.Suggestion = suggestFor(oa.StageStatus[oa.CollapseStage])
	case collapsed && authored > 15 && oa.CollapseStage == "":
		oa.CollapseStage = StagePanned
		oa.StageStatus[StagePanned] = fmt.Sprintf(
			"布局 %s 表达能力不足：创作半径 %.1f° 的对象 %.0f%% 能量落入中置",
			res.Layout.Name, authored, oa.CenterRatio*100)
		oa.Suggestion = "改用 4+5+0 等更宽布局重渲，或人工将该对象重新定位到 Ls/Rs"
	case collapsed && authored > 15:
		oa.Suggestion = suggestFor(oa.StageStatus[oa.CollapseStage])
	}
	if oa.Unrenderable && oa.Suggestion == "" {
		oa.Suggestion = suggestFor(oa.StageStatus[oa.CollapseStage])
	}
	if oa.Suggestion == "" && oa.CollapseStage != "" {
		oa.Suggestion = suggestFor(oa.StageStatus[oa.CollapseStage])
	}
}

func suggestFor(detail string) string {
	switch {
	case strings.Contains(detail, "坐标系不匹配"):
		return "坐标系不匹配：在导出端统一 coordinateType（建议 polar），或对 cartesian 块人工换算为 az/el 后重写 ADM"
	case strings.Contains(detail, "缺少 position"):
		return "缺少 position：人工补写 audioBlockFormat 的 azimuth/elevation，不要依赖默认 (0°,0°)"
	case strings.Contains(detail, "无高度声道"):
		return "高度信息丢失：改用 4+5+0 布局，或人工把顶部对象下移到 ±30° 以内的环绕层"
	case strings.Contains(detail, "不存在声道"):
		return "DirectSpeakers 声道不在目标布局：人工把该声道改送到布局中最近的声道，或改按 Objects 类型重渲"
	case strings.Contains(detail, "speakerLabel"):
		return "DirectSpeakers 标签无法映射：人工核对标签表，或改按 Objects 类型重渲该声道"
	case strings.Contains(detail, "LFE"):
		return "LFE 能量被丢弃：下混前人工将 LFE 混入主声道，或保留含 LFE 的布局"
	case strings.Contains(detail, "screenEdgeLock"):
		return "screenEdgeLock 钳制：确认是否有意为之；如需宽声像，移除 screenEdgeLock 后重导"
	default:
		return "人工复核该对象：" + detail
	}
}

func centerRatio(energy []float64, layout Layout) float64 {
	var total, c float64
	for i, s := range layout.Speakers {
		total += energy[i]
		if s.Short == "C" {
			c += energy[i]
		}
	}
	if total == 0 {
		return 0
	}
	if c > 0 {
		return c / total
	}
	// No centre channel: balanced L/R counts as centred.
	var l, r float64
	for i, s := range layout.Speakers {
		if s.Short == "L" {
			l = energy[i]
		}
		if s.Short == "R" {
			r = energy[i]
		}
	}
	if l+r == 0 {
		return 0
	}
	return 1 - math.Abs(l-r)/(l+r)
}

func spreadDeg(energy []float64, layout Layout) float64 {
	var total, cx, cy float64
	for i, s := range layout.Speakers {
		if s.LFE {
			continue
		}
		total += energy[i]
		v := unitVec(s.Azimuth, s.Elevation)
		cx += energy[i] * v[0]
		cy += energy[i] * v[1]
	}
	if total == 0 {
		return 0
	}
	// Resultant length R in [0,1]; convert to an angular spread estimate.
	r := math.Hypot(cx, cy) / total
	return math.Acos(math.Min(1, r)) * 180 / math.Pi
}
