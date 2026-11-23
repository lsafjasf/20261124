package fcpxml

import (
	"fmt"
	"math/big"
)

type LossPoint struct {
	Position   *FramePosition `json:"position,omitempty"`
	Code       string         `json:"code"`
	Category   Category       `json:"category"`
	Message    string         `json:"message"`
	TrackID    string         `json:"trackId,omitempty"`
	ItemID     string         `json:"itemId,omitempty"`
	EffectID   string         `json:"effectId,omitempty"`
	MediaRef   string         `json:"mediaRef,omitempty"`
	SourcePath string         `json:"sourcePath,omitempty"`
}

type Comparison struct {
	Status            string     `json:"status"`
	Compatible        bool       `json:"compatible"`
	Source            *Inventory `json:"source"`
	Candidate         *Inventory `json:"candidate"`
	Losses            []Finding  `json:"losses"`
	Warnings          []Finding  `json:"warnings,omitempty"`
	SourceIssues      []Finding  `json:"sourceIssues,omitempty"`
	CandidateIssues   []Finding  `json:"candidateIssues,omitempty"`
	EarliestLoss      *LossPoint `json:"earliestLoss,omitempty"`
	LostTrackCount    int        `json:"lostTrackCount"`
	LostItemCount     int        `json:"lostItemCount"`
	LostEffectCount   int        `json:"lostEffectCount"`
	LostMediaRefCount int        `json:"lostMediaReferenceCount"`
}

func CompareDocuments(source, candidate *Document) (*Comparison, error) {
	sourceInventory, err := source.Inventory()
	if err != nil {
		return nil, err
	}
	candidateInventory, err := candidate.Inventory()
	if err != nil {
		return nil, err
	}
	return CompareInventories(sourceInventory, candidateInventory), nil
}

func CompareInventories(source, candidate *Inventory) *Comparison {
	result := &Comparison{
		Source:    source,
		Candidate: candidate,
	}
	if source == nil || candidate == nil {
		result.Status = "invalid"
		return result
	}
	for _, finding := range source.Findings {
		if finding.Severity == SeverityLoss || finding.Severity == SeverityWarning {
			result.SourceIssues = append(result.SourceIssues, finding)
		}
	}
	for _, finding := range candidate.Findings {
		if finding.Severity == SeverityLoss || finding.Severity == SeverityWarning {
			result.CandidateIssues = append(result.CandidateIssues, finding)
		}
		if finding.Severity == SeverityLoss {
			copied := finding
			copied.Message = "candidate: " + copied.Message
			result.Losses = append(result.Losses, copied)
		}
	}

	trackPairs := pairTracks(source.Tracks, candidate.Tracks)
	for _, track := range source.Tracks {
		candidateTrack, ok := trackPairs[track.ID]
		if !ok {
			result.LostTrackCount++
			finding := Finding{
				Code:       "TRACK_MISSING",
				Severity:   SeverityLoss,
				Category:   CategoryTrack,
				Message:    fmt.Sprintf("track %s (%s lane %d) is absent from the candidate", track.ID, track.Sequence, track.Lane),
				TrackID:    track.ID,
				Position:   earliestTrackPosition(source, track.ID),
				SourcePath: track.Path,
			}
			result.Losses = append(result.Losses, finding)
			continue
		}
		if track.FrameDuration != "" && candidateTrack.FrameDuration != "" &&
			track.FrameDuration != candidateTrack.FrameDuration {
			result.Losses = append(result.Losses, Finding{
				Code:       "SEQUENCE_FRAME_RATE_CHANGED",
				Severity:   SeverityLoss,
				Category:   CategoryFrame,
				Message:    fmt.Sprintf("frame duration changed from %s to %s", track.FrameDuration, candidateTrack.FrameDuration),
				TrackID:    track.ID,
				Position:   sequenceStartPosition(track.FrameDuration),
				SourcePath: track.Path,
			})
		}
		if track.SequenceDuration != candidateTrack.SequenceDuration &&
			(track.SequenceDuration != "" || candidateTrack.SequenceDuration != "") {
			position := trackTimePosition(track.SequenceDuration, track.FrameDuration)
			if position == nil {
				position = sequenceStartPosition(track.FrameDuration)
			}
			result.Losses = append(result.Losses, Finding{
				Code:       "SEQUENCE_DURATION_CHANGED",
				Severity:   SeverityLoss,
				Category:   CategoryFrame,
				Message:    fmt.Sprintf("sequence duration changed from %s to %s seconds", track.SequenceDuration, candidateTrack.SequenceDuration),
				TrackID:    track.ID,
				Position:   position,
				SourcePath: track.Path,
			})
		}
		sourceItems := itemsForTrack(source, track.ID)
		candidateItems := itemsForTrack(candidate, candidateTrack.ID)
		result.LostItemCount += compareItems(sourceItems, candidateItems, result, track.ID, candidateTrack.ID)
		sourceEffects := effectsForTrack(source, track.ID)
		candidateEffects := effectsForTrack(candidate, candidateTrack.ID)
		result.LostEffectCount += compareEffects(sourceEffects, candidateEffects, result, track.ID)
	}
	result.LostMediaRefCount = compareMediaReferences(source, candidate, result)

	sortLosses(result.Losses)
	result.EarliestLoss = earliestLoss(result.Losses)
	result.Compatible = len(result.Losses) == 0
	switch {
	case hasLoss(result.SourceIssues):
		result.Status = "invalid"
	case result.Compatible:
		result.Status = "compatible"
	default:
		result.Status = "lossy"
	}
	return result
}

func sequenceStartPosition(frameDuration string) *FramePosition {
	parsed, err := ParseTime(frameDuration)
	if err != nil {
		return nil
	}
	return framePosition(new(big.Rat), parsed.Rat())
}

func trackTimePosition(seconds, frameDuration string) *FramePosition {
	timeValue, err := ParseTime(seconds)
	if err != nil {
		return nil
	}
	var frame *big.Rat
	if frameDuration != "" {
		if parsed, err := ParseTime(frameDuration); err == nil {
			frame = parsed.Rat()
		}
	}
	return framePosition(timeValue.Rat(), frame)
}

func pairTracks(source, candidate []TrackInfo) map[string]TrackInfo {
	pairs := make(map[string]TrackInfo)
	used := make(map[string]bool)
	for _, sourceTrack := range source {
		for _, candidateTrack := range candidate {
			if used[candidateTrack.ID] || candidateTrack.Key != sourceTrack.Key {
				continue
			}
			pairs[sourceTrack.ID] = candidateTrack
			used[candidateTrack.ID] = true
			break
		}
	}
	for _, sourceTrack := range source {
		if _, ok := pairs[sourceTrack.ID]; ok {
			continue
		}
		for _, candidateTrack := range candidate {
			if used[candidateTrack.ID] {
				continue
			}
			if candidateTrack.Sequence == sourceTrack.Sequence && candidateTrack.Lane == sourceTrack.Lane {
				pairs[sourceTrack.ID] = candidateTrack
				used[candidateTrack.ID] = true
				break
			}
		}
	}
	sourceSequences := trackSequenceKeySet(source)
	candidateSequences := trackSequenceKeySet(candidate)
	if len(sourceSequences) == 1 && len(candidateSequences) == 1 {
		for _, sourceTrack := range source {
			if _, ok := pairs[sourceTrack.ID]; ok {
				continue
			}
			for _, candidateTrack := range candidate {
				if !used[candidateTrack.ID] && candidateTrack.Lane == sourceTrack.Lane {
					pairs[sourceTrack.ID] = candidateTrack
					used[candidateTrack.ID] = true
					break
				}
			}
		}
	}
	return pairs
}

func trackSequenceKeySet(tracks []TrackInfo) map[string]bool {
	result := make(map[string]bool)
	for _, track := range tracks {
		result[track.Sequence] = true
	}
	return result
}

func itemsForTrack(inventory *Inventory, trackID string) []*ItemInfo {
	var items []*ItemInfo
	for _, item := range inventory.Items {
		if item.TrackID == trackID && item.Kind != "transition" {
			items = append(items, item)
		}
	}
	return items
}

func effectsForTrack(inventory *Inventory, trackID string) []*EffectInfo {
	var effects []*EffectInfo
	for _, effect := range inventory.Effects {
		if effect.TrackID == trackID {
			effects = append(effects, effect)
		}
	}
	return effects
}

func earliestTrackPosition(inventory *Inventory, trackID string) *FramePosition {
	var earliest *FramePosition
	for _, item := range inventory.Items {
		if item.TrackID == trackID && (earliest == nil || positionLess(item.Offset, earliest)) {
			earliest = item.Offset
		}
	}
	for _, effect := range inventory.Effects {
		if effect.TrackID == trackID && (earliest == nil || positionLess(effect.Offset, earliest)) {
			earliest = effect.Offset
		}
	}
	return earliest
}

func hasLoss(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity == SeverityLoss {
			return true
		}
	}
	return false
}
