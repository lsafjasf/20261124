package fcpxml

import (
	"fmt"
	"math/big"
	"sort"
)

type itemMatch struct {
	index    int
	score    int
	distance *big.Rat
}

func compareItems(source, candidate []*ItemInfo, result *Comparison, sourceTrackID, candidateTrackID string) int {
	used := make(map[int]bool)
	lost := 0
	for _, sourceItem := range source {
		match, ok := bestItemMatch(sourceItem, candidate, used)
		if !ok {
			lost++
			result.Losses = append(result.Losses, Finding{
				Code:       "MEDIA_ITEM_MISSING",
				Severity:   SeverityLoss,
				Category:   CategoryMedia,
				Message:    fmt.Sprintf("%s item %q is absent from the candidate", sourceItem.Kind, itemLabel(sourceItem)),
				TrackID:    sourceTrackID,
				ItemID:     sourceItem.ID,
				MediaRef:   sourceItem.Ref,
				Position:   sourceItem.Offset,
				SourcePath: sourceItem.Path,
			})
			continue
		}
		used[match.index] = true
		candidateItem := candidate[match.index]
		lost += compareItemFields(sourceItem, candidateItem, result, sourceTrackID)
	}
	for index, candidateItem := range candidate {
		if used[index] {
			continue
		}
		result.Warnings = append(result.Warnings, Finding{
			Code:       "EXTRA_ITEM",
			Severity:   SeverityWarning,
			Category:   CategoryMedia,
			Message:    fmt.Sprintf("candidate contains an extra %s item %q", candidateItem.Kind, itemLabel(candidateItem)),
			TrackID:    candidateTrackID,
			ItemID:     candidateItem.ID,
			Position:   candidateItem.Offset,
			TargetPath: candidateItem.Path,
		})
	}
	return lost
}

func bestItemMatch(source *ItemInfo, candidate []*ItemInfo, used map[int]bool) (itemMatch, bool) {
	best := itemMatch{index: -1, score: 100}
	for index, current := range candidate {
		if used[index] || current.Kind != source.Kind {
			continue
		}
		score := itemMatchScore(source, current)
		if score > 4 {
			continue
		}
		distance := positionDistance(source.Offset, current.Offset)
		if best.index < 0 || score < best.score ||
			(score == best.score && ratLess(distance, best.distance)) {
			best = itemMatch{index: index, score: score, distance: distance}
		}
	}
	return best, best.index >= 0
}

func itemMatchScore(source, candidate *ItemInfo) int {
	mediaEqual := source.MediaKey != "" && source.MediaKey == candidate.MediaKey
	sourceEqual := equalPosition(source.SourceIn, candidate.SourceIn)
	offsetEqual := equalPosition(source.Offset, candidate.Offset)
	switch {
	case mediaEqual && sourceEqual:
		return 0
	case mediaEqual:
		return 1
	case sourceEqual && offsetEqual:
		return 2
	case sourceEqual:
		return 3
	case offsetEqual:
		return 4
	default:
		return 5
	}
}

func compareItemFields(source, candidate *ItemInfo, result *Comparison, sourceTrackID string) int {
	lost := 0
	if source.MediaKey != candidate.MediaKey {
		lost++
		result.Losses = append(result.Losses, Finding{
			Code:       "MEDIA_REFERENCE_CHANGED",
			Severity:   SeverityLoss,
			Category:   CategoryMedia,
			Message:    fmt.Sprintf("media reference changed from %q to %q", source.MediaKey, candidate.MediaKey),
			TrackID:    sourceTrackID,
			ItemID:     source.ID,
			MediaRef:   source.Ref,
			Position:   source.Offset,
			SourcePath: source.Path,
			TargetPath: candidate.Path,
		})
	}
	if !equalPosition(source.Offset, candidate.Offset) {
		lost++
		result.Losses = append(result.Losses, Finding{
			Code:       "FRAME_POSITION_CHANGED",
			Severity:   SeverityLoss,
			Category:   CategoryFrame,
			Message:    fmt.Sprintf("offset changed from %s to %s", positionSummary(source.Offset), positionSummary(candidate.Offset)),
			TrackID:    sourceTrackID,
			ItemID:     source.ID,
			Position:   source.Offset,
			SourcePath: source.Path,
			TargetPath: candidate.Path,
		})
	}
	if !equalPosition(source.Duration, candidate.Duration) {
		lost++
		result.Losses = append(result.Losses, Finding{
			Code:       "DURATION_CHANGED",
			Severity:   SeverityLoss,
			Category:   CategoryFrame,
			Message:    fmt.Sprintf("duration changed from %s to %s", positionSummary(source.Duration), positionSummary(candidate.Duration)),
			TrackID:    sourceTrackID,
			ItemID:     source.ID,
			Position:   source.Offset,
			SourcePath: source.Path,
			TargetPath: candidate.Path,
		})
	}
	if !equalPosition(source.SourceIn, candidate.SourceIn) {
		lost++
		result.Losses = append(result.Losses, Finding{
			Code:       "MEDIA_RANGE_CHANGED",
			Severity:   SeverityLoss,
			Category:   CategoryMedia,
			Message:    fmt.Sprintf("source in-point changed from %s to %s", positionSummary(source.SourceIn), positionSummary(candidate.SourceIn)),
			TrackID:    sourceTrackID,
			ItemID:     source.ID,
			MediaRef:   source.Ref,
			Position:   source.Offset,
			SourcePath: source.Path,
			TargetPath: candidate.Path,
		})
	}
	return lost
}

type effectMatch struct {
	index    int
	score    int
	distance *big.Rat
}

func compareEffects(source, candidate []*EffectInfo, result *Comparison, sourceTrackID string) int {
	used := make(map[int]bool)
	lost := 0
	for _, sourceEffect := range source {
		match, ok := bestEffectMatch(sourceEffect, candidate, used)
		if !ok {
			lost++
			code := "EFFECT_MISSING"
			message := fmt.Sprintf("%s effect %q is absent from the candidate", sourceEffect.Kind, effectLabel(sourceEffect))
			if sourceEffect.Kind == "transition" {
				code = "TRANSITION_FLATTENED"
				message = fmt.Sprintf("transition %q was flattened to a hard cut (offset %s)", effectLabel(sourceEffect), positionSummary(sourceEffect.Offset))
			}
			if sourceEffect.Kind == "time-map" {
				code = "RETIME_EFFECT_MISSING"
				message = fmt.Sprintf("retime/speed effect at %s was dropped", positionSummary(sourceEffect.Offset))
			}
			result.Losses = append(result.Losses, Finding{
				Code:       code,
				Severity:   SeverityLoss,
				Category:   CategoryEffect,
				Message:    message,
				TrackID:    sourceTrackID,
				ItemID:     sourceEffect.ItemID,
				EffectID:   sourceEffect.ID,
				MediaRef:   sourceEffect.Ref,
				Position:   sourceEffect.Offset,
				SourcePath: sourceEffect.Path,
			})
			continue
		}
		used[match.index] = true
		candidateEffect := candidate[match.index]
		lost += compareEffectFields(sourceEffect, candidateEffect, result, sourceTrackID)
	}
	for index, candidateEffect := range candidate {
		if used[index] {
			continue
		}
		result.Warnings = append(result.Warnings, Finding{
			Code:       "EXTRA_EFFECT",
			Severity:   SeverityWarning,
			Category:   CategoryEffect,
			Message:    fmt.Sprintf("candidate contains an extra %s effect %q", candidateEffect.Kind, effectLabel(candidateEffect)),
			TrackID:    sourceTrackID,
			EffectID:   candidateEffect.ID,
			Position:   candidateEffect.Offset,
			TargetPath: candidateEffect.Path,
		})
	}
	return lost
}

func bestEffectMatch(source *EffectInfo, candidate []*EffectInfo, used map[int]bool) (effectMatch, bool) {
	best := effectMatch{index: -1, score: 100}
	for index, current := range candidate {
		if used[index] || current.Kind != source.Kind || current.OwnerKey != source.OwnerKey {
			continue
		}
		score := 4
		switch {
		case source.Identity != "" && source.Identity == current.Identity:
			score = 0
		case source.ResourceKey != "" && source.ResourceKey == current.ResourceKey:
			score = 1
		case source.Name != "" && source.Name == current.Name:
			score = 2
		case equalPosition(source.Offset, current.Offset):
			score = 3
		}
		distance := positionDistance(source.Offset, current.Offset)
		if best.index < 0 || score < best.score ||
			(score == best.score && ratLess(distance, best.distance)) {
			best = effectMatch{index: index, score: score, distance: distance}
		}
	}
	return best, best.index >= 0
}

func compareEffectFields(source, candidate *EffectInfo, result *Comparison, sourceTrackID string) int {
	lost := 0
	add := func(code, message string) {
		lost++
		result.Losses = append(result.Losses, Finding{
			Code:       code,
			Severity:   SeverityLoss,
			Category:   CategoryEffect,
			Message:    message,
			TrackID:    sourceTrackID,
			ItemID:     source.ItemID,
			EffectID:   source.ID,
			MediaRef:   source.Ref,
			Position:   source.Offset,
			SourcePath: source.Path,
			TargetPath: candidate.Path,
		})
	}
	if source.ResourceKey != candidate.ResourceKey {
		add("EFFECT_REFERENCE_CHANGED", fmt.Sprintf("effect resource changed from %q to %q", source.ResourceKey, candidate.ResourceKey))
	}
	if !equalPosition(source.Offset, candidate.Offset) {
		add("EFFECT_POSITION_CHANGED", fmt.Sprintf("effect offset changed from %s to %s", positionSummary(source.Offset), positionSummary(candidate.Offset)))
	}
	if !equalPosition(source.Duration, candidate.Duration) {
		add("EFFECT_DURATION_CHANGED", fmt.Sprintf("effect duration changed from %s to %s", positionSummary(source.Duration), positionSummary(candidate.Duration)))
	}
	if source.Kind == "time-map" {
		sourceSpeed, sourceOK := new(big.Rat).SetString(source.Speed)
		candidateSpeed, candidateOK := new(big.Rat).SetString(candidate.Speed)
		if sourceOK && candidateOK && sourceSpeed.Cmp(candidateSpeed) != 0 {
			add("SPEED_CHANGED", fmt.Sprintf("speed changed from %s to %s", source.Speed, candidate.Speed))
		}
	}
	if source.Fingerprint != candidate.Fingerprint {
		add("EFFECT_PARAMETER_CHANGED", "effect parameters/keyframes differ")
	}
	return lost
}

func itemLabel(item *ItemInfo) string {
	if item.Name != "" {
		return item.Name
	}
	if item.Ref != "" {
		return item.Ref
	}
	return item.Path
}

func effectLabel(effect *EffectInfo) string {
	if effect.Name != "" {
		return effect.Name
	}
	if effect.ResourceName != "" {
		return effect.ResourceName
	}
	if effect.Ref != "" {
		return effect.Ref
	}
	return effect.Kind
}

func equalPosition(a, b *FramePosition) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.seconds == nil || b.seconds == nil {
		return a.Seconds == b.Seconds
	}
	return a.seconds.Cmp(b.seconds) == 0
}

func positionDistance(a, b *FramePosition) *big.Rat {
	if a == nil || b == nil || a.seconds == nil || b.seconds == nil {
		return nil
	}
	distance := new(big.Rat).Sub(a.seconds, b.seconds)
	return distance.Abs(distance)
}

func ratLess(a, b *big.Rat) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.Cmp(b) < 0
}

func positionSummary(position *FramePosition) string {
	if position == nil {
		return "<unknown>"
	}
	if position.FrameExact != "" {
		return fmt.Sprintf("%ss (frame %s)", position.Seconds, position.FrameExact)
	}
	return position.Seconds + "s"
}

func sortLosses(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if !equalPosition(a.Position, b.Position) {
			if a.Position == nil {
				return false
			}
			if b.Position == nil {
				return true
			}
			return positionLess(a.Position, b.Position)
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if lossCodePriority(a.Code) != lossCodePriority(b.Code) {
			return lossCodePriority(a.Code) < lossCodePriority(b.Code)
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.TrackID != b.TrackID {
			return a.TrackID < b.TrackID
		}
		return a.SourcePath < b.SourcePath
	})
}

func lossCodePriority(code string) int {
	switch code {
	case "TRACK_MISSING":
		return 0
	case "TRANSITION_FLATTENED":
		return 1
	case "RETIME_EFFECT_MISSING":
		return 2
	case "MEDIA_REFERENCE_CHANGED":
		return 3
	case "FRAME_POSITION_CHANGED":
		return 4
	case "EFFECT_MISSING":
		return 5
	default:
		return 10
	}
}

func earliestLoss(findings []Finding) *LossPoint {
	if len(findings) == 0 {
		return nil
	}
	first := findings[0]
	return &LossPoint{
		Position:   first.Position,
		Code:       first.Code,
		Category:   first.Category,
		Message:    first.Message,
		TrackID:    first.TrackID,
		ItemID:     first.ItemID,
		EffectID:   first.EffectID,
		MediaRef:   first.MediaRef,
		SourcePath: first.SourcePath,
	}
}
