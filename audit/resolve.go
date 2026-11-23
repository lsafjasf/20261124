package audit

import (
	"fmt"
	"strings"

	"hdr10audit/hdr"
)

type sourceResolution struct {
	mastering          *hdr.Mastering
	masteringPresent   bool
	masteringAmbiguous bool
	light              *hdr.ContentLight
	lightPresent       bool
	lightAmbiguous     bool
}

func resolveSource(occs []hdr.Occurrence, source hdr.Source) (sourceResolution, []hdr.Issue) {
	var out sourceResolution
	var issues []hdr.Issue
	var masters []hdr.Mastering
	var masterCount int
	var lights []hdr.ContentLight
	var lightCount int
	for _, occ := range occs {
		if occ.Metadata.Mastering != nil {
			out.masteringPresent = true
			masterCount += max(1, occ.Count)
			if !containsMastering(masters, *occ.Metadata.Mastering) {
				masters = append(masters, *occ.Metadata.Mastering)
			}
		}
		if occ.Metadata.Light != nil {
			out.lightPresent = true
			lightCount += max(1, occ.Count)
			if !containsLight(lights, *occ.Metadata.Light) {
				lights = append(lights, *occ.Metadata.Light)
			}
		}
	}
	if len(masters) == 1 {
		out.mastering = &masters[0]
	}
	if len(masters) > 1 {
		out.masteringAmbiguous = true
		issues = append(issues, hdr.Issue{
			Code: "duplicate_field_conflict", Severity: hdr.SeverityError,
			Source: source, Field: "mastering_display",
			Message: fmt.Sprintf("%s 内出现 %d 组不同的 MDCV 值：%s",
				source, len(masters), describeMasters(masters)),
		})
	}
	if len(lights) == 1 {
		out.light = &lights[0]
	}
	if len(lights) > 1 {
		out.lightAmbiguous = true
		issues = append(issues, hdr.Issue{
			Code: "duplicate_field_conflict", Severity: hdr.SeverityError,
			Source: source, Field: "content_light",
			Message: fmt.Sprintf("%s 内出现 %d 组不同的 CLLI 值：%s",
				source, len(lights), describeLights(lights)),
		})
	}
	if source == hdr.SourceContainer && len(masters) == 1 && masterCount > 1 {
		issues = append(issues, duplicateIssue(source, "mastering_display", masterCount))
	}
	if source == hdr.SourceContainer && len(lights) == 1 && lightCount > 1 {
		issues = append(issues, duplicateIssue(source, "content_light", lightCount))
	}
	if source == hdr.SourceBitstream && masterCount > 1 && len(masters) == 1 {
		issues = append(issues, hdr.Issue{
			Code: "repeated_metadata_consistent", Severity: hdr.SeverityInfo,
			Source: source, Field: "mastering_display",
			Message: fmt.Sprintf("MDCV 在码流中重复 %d 次且值一致", masterCount),
		})
	}
	if source == hdr.SourceBitstream && lightCount > 1 && len(lights) == 1 {
		issues = append(issues, hdr.Issue{
			Code: "repeated_metadata_consistent", Severity: hdr.SeverityInfo,
			Source: source, Field: "content_light",
			Message: fmt.Sprintf("CLLI 在码流中重复 %d 次且值一致", lightCount),
		})
	}
	return out, issues
}

func chooseMastering(container, stream sourceResolution) (*hdr.Mastering, string, []hdr.Issue) {
	if stream.mastering != nil {
		return stream.mastering, "bitstream", nil
	}
	if stream.masteringAmbiguous {
		return nil, "", nil
	}
	if container.mastering != nil {
		return container.mastering, "container", []hdr.Issue{fallbackIssue("mastering_display")}
	}
	return nil, "", nil
}

func chooseLight(container, stream sourceResolution) (*hdr.ContentLight, string, []hdr.Issue) {
	if stream.light != nil {
		return stream.light, "bitstream", nil
	}
	if stream.lightAmbiguous {
		return nil, "", nil
	}
	if container.light != nil {
		return container.light, "container", []hdr.Issue{fallbackIssue("content_light")}
	}
	return nil, "", nil
}

func missingIssues(container, stream sourceResolution) []hdr.Issue {
	var out []hdr.Issue
	add := func(code string, sev hdr.Severity, source hdr.Source, field, msg string) {
		out = append(out, hdr.Issue{
			Code: code, Severity: sev, Source: source, Field: field, Message: msg,
		})
	}
	if !container.masteringPresent {
		add("missing_container_metadata", hdr.SeverityWarning, hdr.SourceContainer,
			"mastering_display", "容器缺少 mdcv（母版显示色彩体积）")
	}
	if !stream.masteringPresent {
		add("missing_bitstream_metadata", hdr.SeverityError, hdr.SourceBitstream,
			"mastering_display", "码流缺少 MDCV SEI（母版显示色彩体积）")
	}
	if !container.lightPresent {
		add("missing_container_metadata", hdr.SeverityWarning, hdr.SourceContainer,
			"content_light", "容器缺少 clli（MaxCLL/MaxFALL）")
	}
	if !stream.lightPresent {
		add("missing_bitstream_metadata", hdr.SeverityError, hdr.SourceBitstream,
			"content_light", "码流缺少 CLLI SEI（MaxCLL/MaxFALL）")
	}
	if !container.masteringPresent && !stream.masteringPresent {
		add("hdr10_metadata_missing", hdr.SeverityError, "", "mastering_display",
			"容器和码流均无 MDCV 元数据")
	}
	if !container.lightPresent && !stream.lightPresent {
		add("hdr10_metadata_missing", hdr.SeverityError, "", "content_light",
			"容器和码流均无 CLLI 元数据")
	}
	return out
}

func compareSources(container, stream sourceResolution) []Conflict {
	var out []Conflict
	if container.mastering != nil && stream.mastering != nil {
		cv := container.mastering.Values()
		sv := stream.mastering.Values()
		for i := range cv {
			if cv[i].Raw != sv[i].Raw {
				out = append(out, Conflict{
					Field: cv[i].Field, Container: cv[i].Raw, Bitstream: sv[i].Raw,
					Winner: "bitstream", Reason: precedenceReason,
				})
			}
		}
	}
	if container.light != nil && stream.light != nil {
		cv := container.light.Values()
		sv := stream.light.Values()
		for i := range cv {
			if cv[i].Raw != sv[i].Raw {
				out = append(out, Conflict{
					Field: cv[i].Field, Container: cv[i].Raw, Bitstream: sv[i].Raw,
					Winner: "bitstream", Reason: precedenceReason,
				})
			}
		}
	}
	return out
}

const precedenceReason = "码流内的 SEI 是解码端实际消费的 HDR10 静态元数据，容器 mdcv/clli 仅为索引副本；冲突时以码流为准"

func fallbackIssue(field string) hdr.Issue {
	return hdr.Issue{
		Code: "container_fallback", Severity: hdr.SeverityInfo,
		Source: hdr.SourceContainer, Field: field,
		Message: "码流缺少该字段，审计值回退到容器副本",
	}
}

func duplicateIssue(source hdr.Source, field string, count int) hdr.Issue {
	return hdr.Issue{
		Code: "duplicate_metadata", Severity: hdr.SeverityWarning,
		Source: source, Field: field,
		Message: fmt.Sprintf("同一 %s 内出现 %d 个相同语义的重复字段", source, count),
	}
}

func containsMastering(values []hdr.Mastering, v hdr.Mastering) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func containsLight(values []hdr.ContentLight, v hdr.ContentLight) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func describeMasters(values []hdr.Mastering) string {
	parts := make([]string, 0, len(values))
	for i := range values {
		parts = append(parts, formatMastering(&values[i]))
	}
	return strings.Join(parts, " | ")
}

func describeLights(values []hdr.ContentLight) string {
	parts := make([]string, 0, len(values))
	for i := range values {
		parts = append(parts, formatLight(&values[i]))
	}
	return strings.Join(parts, " | ")
}
