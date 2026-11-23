package fcpxml

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
)

func (b *inventoryBuilder) collectEffects(seq *sequenceInfo, item *ItemInfo, node *Element) {
	if node.Name.Local == "transition" {
		b.addTransitionEffect(seq, item, node)
	}
	b.walkEffectNodes(node, item, func(effectNode *Element) {
		switch effectNode.Name.Local {
		case "filter-video", "filter-audio", "filter":
			b.addFilterEffect(seq, item, effectNode)
		case "timeMap":
			b.addTimeMapEffect(seq, item, effectNode)
		case "conform-rate":
			b.addConformRateEffect(seq, item, effectNode)
		}
	})
}

func (b *inventoryBuilder) walkEffectNodes(node *Element, owner *ItemInfo, fn func(*Element)) {
	for _, child := range node.Children {
		if isTimelineElement(child.Name.Local) || child.Name.Local == "spine" {
			continue
		}
		switch child.Name.Local {
		case "filter-video", "filter-audio", "filter", "timeMap", "conform-rate":
			fn(child)
			continue
		}
		b.walkEffectNodes(child, owner, fn)
	}
}

func (b *inventoryBuilder) addTransitionEffect(seq *sequenceInfo, item *ItemInfo, node *Element) {
	ref := firstEffectRef(node)
	effect := &EffectInfo{
		ID:       node.Path + "#transition",
		Path:     node.Path,
		TrackID:  item.TrackID,
		ItemID:   item.ID,
		Kind:     "transition",
		Name:     cleanName(node.AttrDefault("name", "")),
		Ref:      ref,
		OwnerKey: item.OwnerKey,
		Node:     node,
		Fingerprint: effectFingerprint(node, map[string]bool{
			"id": true, "offset": true, "duration": true, "lane": true,
		}),
	}
	effect.Offset, _ = b.parseAttrTime(node, "offset", seq.FrameDuration)
	effect.Duration, _ = b.parseAttrTime(node, "duration", seq.FrameDuration)
	b.resolveEffectResource(effect, item)
	effect.Identity = "transition|" + effect.Name + "|" + effect.ResourceKey + "|" + effect.OwnerKey
	b.appendEffect(seq, effect)
}

func (b *inventoryBuilder) addFilterEffect(seq *sequenceInfo, item *ItemInfo, node *Element) {
	effect := &EffectInfo{
		ID:          node.Path + "#" + node.Name.Local,
		Path:        node.Path,
		TrackID:     item.TrackID,
		ItemID:      item.ID,
		Kind:        node.Name.Local,
		Name:        cleanName(node.AttrDefault("name", "")),
		Ref:         node.AttrDefault("ref", ""),
		OwnerKey:    item.OwnerKey,
		Offset:      item.Offset,
		Duration:    item.Duration,
		Node:        node,
		Fingerprint: effectFingerprint(node, map[string]bool{"id": true, "ref": true, "name": true}),
	}
	b.resolveEffectResource(effect, item)
	if effect.Name == "" && effect.ResourceName != "" {
		effect.Name = effect.ResourceName
	}
	effect.Identity = effect.Kind + "|" + effect.Name + "|" + effect.ResourceKey + "|" + effect.OwnerKey
	b.appendEffect(seq, effect)
}

func (b *inventoryBuilder) addTimeMapEffect(seq *sequenceInfo, item *ItemInfo, node *Element) {
	effect := &EffectInfo{
		ID:       node.Path + "#timeMap",
		Path:     node.Path,
		TrackID:  item.TrackID,
		ItemID:   item.ID,
		Kind:     "time-map",
		Name:     cleanName(node.AttrDefault("name", "")),
		OwnerKey: item.OwnerKey,
		Offset:   item.Offset,
		Duration: item.Duration,
		Node:     node,
	}
	points, speed, err := parseTimeMap(node)
	effect.TimePoints = points
	if err != nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "TIMEMAP_INVALID",
			Severity: SeverityLoss,
			Category: CategoryEffect,
			Message:  err.Error(),
			TrackID:  item.TrackID,
			ItemID:   item.ID,
			EffectID: effect.ID,
			Path:     node.Path,
			Position: item.Offset,
		})
	} else if speed != nil {
		effect.Speed = RatString(speed)
	}
	effect.Fingerprint = effectFingerprint(node, map[string]bool{"id": true, "name": true})
	effect.Identity = "time-map|" + effect.OwnerKey
	b.appendEffect(seq, effect)
}

func (b *inventoryBuilder) addConformRateEffect(seq *sequenceInfo, item *ItemInfo, node *Element) {
	effect := &EffectInfo{
		ID:          node.Path + "#conform-rate",
		Path:        node.Path,
		TrackID:     item.TrackID,
		ItemID:      item.ID,
		Kind:        "conform-rate",
		Name:        cleanName(node.AttrDefault("name", "")),
		OwnerKey:    item.OwnerKey,
		Offset:      item.Offset,
		Duration:    item.Duration,
		Node:        node,
		Fingerprint: effectFingerprint(node, map[string]bool{"id": true}),
	}
	effect.Identity = "conform-rate|" + effect.OwnerKey
	b.appendEffect(seq, effect)
}

func (b *inventoryBuilder) resolveEffectResource(effect *EffectInfo, item *ItemInfo) {
	if effect.Ref == "" {
		return
	}
	resource := b.index.Resources[effect.Ref]
	if resource == nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "EFFECT_REF_UNRESOLVED",
			Severity: SeverityLoss,
			Category: CategoryEffect,
			Message:  fmt.Sprintf("effect ref %q does not resolve", effect.Ref),
			TrackID:  item.TrackID,
			ItemID:   item.ID,
			EffectID: effect.ID,
			MediaRef: effect.Ref,
			Path:     effect.Path,
			Position: effect.Offset,
		})
		return
	}
	if resource.Type != "effect" {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "EFFECT_REF_TYPE_MISMATCH",
			Severity: SeverityLoss,
			Category: CategoryEffect,
			Message:  fmt.Sprintf("effect ref %q resolves to %s, want effect", effect.Ref, resource.Type),
			TrackID:  item.TrackID,
			ItemID:   item.ID,
			EffectID: effect.ID,
			MediaRef: effect.Ref,
			Path:     effect.Path,
			Position: effect.Offset,
		})
		return
	}
	effect.Resolved = true
	effect.ResourceKey = resource.Key
	effect.ResourceType = resource.Type
	effect.ResourceName = resource.Name
}

func (b *inventoryBuilder) appendEffect(seq *sequenceInfo, effect *EffectInfo) {
	b.inventory.Effects = append(b.inventory.Effects, effect)
	b.effectByID[effect.ID] = effect
}

func firstEffectRef(node *Element) string {
	if ref := node.AttrDefault("ref", ""); ref != "" {
		return ref
	}
	var ref string
	_ = node.Walk(func(current *Element) error {
		if ref != "" {
			return nil
		}
		switch current.Name.Local {
		case "filter-video", "filter-audio", "filter", "effect":
			ref = current.AttrDefault("ref", "")
		}
		return nil
	})
	return ref
}

func parseTimeMap(node *Element) ([]TimePointInfo, *big.Rat, error) {
	var points []TimePointInfo
	var firstTime, firstValue, lastTime, lastValue *big.Rat
	for _, point := range node.ChildrenNamed("timept") {
		rawTime := point.AttrDefault("time", "")
		rawValue := point.AttrDefault("value", "")
		timeValue, err := ParseTime(rawTime)
		if err != nil {
			return points, nil, fmt.Errorf("timept time %q: %w", rawTime, err)
		}
		mediaValue, err := ParseTime(rawValue)
		if err != nil {
			return points, nil, fmt.Errorf("timept value %q: %w", rawValue, err)
		}
		points = append(points, TimePointInfo{
			Time:   timeValue.String(),
			Value:  mediaValue.String(),
			Interp: point.AttrDefault("interp", ""),
		})
		if firstTime == nil {
			firstTime, firstValue = timeValue.Rat(), mediaValue.Rat()
		}
		lastTime, lastValue = timeValue.Rat(), mediaValue.Rat()
	}
	if len(points) < 2 {
		return points, nil, fmt.Errorf("timeMap has %d time points, want at least 2", len(points))
	}
	timeDelta := new(big.Rat).Sub(lastTime, firstTime)
	mediaDelta := new(big.Rat).Sub(lastValue, firstValue)
	if timeDelta.Sign() <= 0 {
		return points, nil, fmt.Errorf("timeMap timeline time does not advance")
	}
	if mediaDelta.Sign() == 0 {
		return points, nil, fmt.Errorf("timeMap media time does not advance")
	}
	return points, new(big.Rat).Quo(timeDelta, mediaDelta), nil
}

func effectFingerprint(node *Element, ignore map[string]bool) string {
	var builder strings.Builder
	var visit func(*Element)
	visit = func(current *Element) {
		builder.WriteString("<")
		builder.WriteString(current.Name.Local)
		attrs := make([]string, 0, len(current.Attr))
		for _, attr := range current.Attr {
			if ignore[attr.Name.Local] || attr.Name.Local == "ref" {
				continue
			}
			attrs = append(attrs, attr.Name.Local+"="+attr.Value)
		}
		sort.Strings(attrs)
		for _, attr := range attrs {
			builder.WriteString(" ")
			builder.WriteString(attr)
		}
		builder.WriteString(">")
		for _, item := range current.Content {
			if item.kind == contentText {
				if text := strings.TrimSpace(item.text); text != "" {
					builder.WriteString(text)
				}
			}
		}
		for _, child := range current.Children {
			visit(child)
		}
		builder.WriteString("</")
		builder.WriteString(current.Name.Local)
		builder.WriteString(">")
	}
	visit(node)
	return builder.String()
}
