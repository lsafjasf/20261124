package fcpxml

import "fmt"

func compareMediaReferences(source, candidate *Inventory, result *Comparison) int {
	used := make(map[int]bool)
	lost := 0
	for _, sourceRef := range source.MediaRefs {
		if !comparableMediaRef(sourceRef) {
			continue
		}
		key := mediaRefKey(sourceRef)
		match := -1
		for index, candidateRef := range candidate.MediaRefs {
			if used[index] || !comparableMediaRef(candidateRef) {
				continue
			}
			if candidateRef.Kind == sourceRef.Kind &&
				candidateRef.OwnerKey == sourceRef.OwnerKey &&
				mediaRefKey(candidateRef) == key {
				match = index
				break
			}
		}
		if match < 0 {
			lost++
			result.Losses = append(result.Losses, Finding{
				Code:       "MEDIA_REFERENCE_MISSING",
				Severity:   SeverityLoss,
				Category:   CategoryMedia,
				Message:    fmt.Sprintf("%s media reference %q is absent from the candidate", sourceRef.Kind, sourceRef.Value),
				TrackID:    sourceRef.TrackID,
				ItemID:     sourceRef.ItemID,
				MediaRef:   sourceRef.Value,
				Position:   sourceRef.Position,
				SourcePath: sourceRef.Path,
			})
			continue
		}
		used[match] = true
	}
	for index, candidateRef := range candidate.MediaRefs {
		if used[index] || !comparableMediaRef(candidateRef) {
			continue
		}
		result.Warnings = append(result.Warnings, Finding{
			Code:       "EXTRA_MEDIA_REFERENCE",
			Severity:   SeverityWarning,
			Category:   CategoryMedia,
			Message:    fmt.Sprintf("candidate contains an extra %s media reference %q", candidateRef.Kind, candidateRef.Value),
			TrackID:    candidateRef.TrackID,
			ItemID:     candidateRef.ItemID,
			MediaRef:   candidateRef.Value,
			Position:   candidateRef.Position,
			TargetPath: candidateRef.Path,
		})
	}
	return lost
}

func comparableMediaRef(ref MediaReferenceInfo) bool {
	switch ref.Kind {
	case "item-ref", "effect-ref", "sequence-format":
		return false
	default:
		return true
	}
}

func mediaRefKey(ref MediaReferenceInfo) string {
	if ref.ResourceKey != "" {
		return "resource:" + ref.ResourceKey
	}
	if ref.URI != "" {
		return "uri:" + ref.URI
	}
	return "value:" + ref.Value
}
