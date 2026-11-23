package hdr

import (
	"fmt"
)

const (
	MaxChromaticityRaw = uint16(50000)
	MaxLuminanceRaw    = uint32(100000000)
	MaxCLLNits         = uint16(10000)
)

type FieldValue struct {
	Field string
	Raw   string
}

func (m Mastering) Values() []FieldValue {
	v := make([]FieldValue, 0, 14)
	for i, name := range []string{"G", "B", "R"} {
		v = append(v,
			FieldValue{fmt.Sprintf("mastering.%s.x", name), fmt.Sprint(m.Primaries[i].X)},
			FieldValue{fmt.Sprintf("mastering.%s.y", name), fmt.Sprint(m.Primaries[i].Y)},
		)
	}
	v = append(v,
		FieldValue{"mastering.white.x", fmt.Sprint(m.WhitePoint.X)},
		FieldValue{"mastering.white.y", fmt.Sprint(m.WhitePoint.Y)},
		FieldValue{"mastering.luminance.max", fmt.Sprint(m.MaxLuminance)},
		FieldValue{"mastering.luminance.min", fmt.Sprint(m.MinLuminance)},
	)
	return v
}

func (c ContentLight) Values() []FieldValue {
	return []FieldValue{
		{"max_cll", fmt.Sprint(c.MaxCLL)},
		{"max_fall", fmt.Sprint(c.MaxFALL)},
	}
}

func (m Metadata) Values() []FieldValue {
	var values []FieldValue
	if m.Mastering != nil {
		values = append(values, m.Mastering.Values()...)
	}
	if m.Light != nil {
		values = append(values, m.Light.Values()...)
	}
	return values
}

func Validate(m Metadata, source Source, location string) []Issue {
	var issues []Issue
	add := func(code string, field, msg string) {
		issues = append(issues, Issue{
			Code: code, Severity: SeverityError, Source: source,
			Field: field, Location: location, Message: msg,
		})
	}
	if md := m.Mastering; md != nil {
		for i, name := range []string{"G", "B", "R"} {
			validatePoint(add, md.Primaries[i], fmt.Sprintf("mastering.%s", name))
		}
		validatePoint(add, md.WhitePoint, "mastering.white")
		if md.MaxLuminance > MaxLuminanceRaw {
			add("luminance_out_of_range", "mastering.luminance.max",
				fmt.Sprintf("最大母版亮度 %.4f cd/m² 超过 HDR10 上限 10000 cd/m²",
					md.MaxNits()))
		}
		if md.MaxLuminance > 0 && md.MaxLuminance < 10000 {
			add("luminance_unit_suspected", "mastering.luminance.max",
				fmt.Sprintf("最大母版亮度仅 %.4f cd/m²，疑似把 cd/m² 直接写入 0.0001 单位字段",
					md.MaxNits()))
		}
		if md.MaxLuminance < md.MinLuminance {
			add("luminance_range_invalid", "mastering.luminance",
				fmt.Sprintf("最大亮度 %.4f cd/m² 小于最小亮度 %.4f cd/m²",
					md.MaxNits(), md.MinNits()))
		}
		if md.MaxLuminance == 0 {
			add("luminance_range_invalid", "mastering.luminance.max",
				"最大母版亮度不能为 0")
		}
		if md.WhitePoint.X > 0 && md.WhitePoint.Y > 0 &&
			(md.WhitePoint.X < 2000 || md.WhitePoint.Y < 2000) {
			add("chromaticity_unit_suspected", "mastering.white",
				fmt.Sprintf("白点坐标 (%.5f,%.5f) 远低于常用显示白点，疑似色度单位缩放错误",
					md.WhitePoint.XValue(), md.WhitePoint.YValue()))
		}
	}
	if cl := m.Light; cl != nil {
		if cl.MaxCLL > MaxCLLNits {
			add("max_cll_out_of_range", "max_cll",
				fmt.Sprintf("MaxCLL=%d cd/m² 超过 HDR10 上限 10000 cd/m²", cl.MaxCLL))
		}
		if cl.MaxFALL > MaxCLLNits {
			add("max_fall_out_of_range", "max_fall",
				fmt.Sprintf("MaxFALL=%d cd/m² 超过 HDR10 上限 10000 cd/m²", cl.MaxFALL))
		}
		if cl.MaxFALL > cl.MaxCLL {
			add("max_fall_exceeds_max_cll", "max_fall",
				fmt.Sprintf("MaxFALL=%d 大于 MaxCLL=%d", cl.MaxFALL, cl.MaxCLL))
		}
	}
	return issues
}

func validatePoint(add func(string, string, string), p Point, field string) {
	if p.X > MaxChromaticityRaw || p.Y > MaxChromaticityRaw {
		add("chromaticity_out_of_range", field,
			fmt.Sprintf("色度原始值 (%d,%d) 超出 [0,50000]，疑似按 0.0001 或百分比编码",
				p.X, p.Y))
	}
}
