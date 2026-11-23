package subselector

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
)

// Kind classifies how a subtitle track is meant to be used.
type Kind int

const (
	// KindNormal is a regular, full-dialogue subtitle track.
	KindNormal Kind = iota
	// KindSDH is subtitles for the deaf and hard-of-hearing.
	KindSDH
	// KindForced only translates foreign-language parts of the audio.
	KindForced
	// KindCommentary carries director/cast commentary, not dialogue.
	KindCommentary
)

func (k Kind) String() string {
	switch k {
	case KindSDH:
		return "sdh"
	case KindForced:
		return "forced"
	case KindCommentary:
		return "commentary"
	default:
		return "normal"
	}
}

// Track describes one selectable subtitle stream.
type Track struct {
	ID          string
	LanguageTag string // BCP 47, e.g. "zh-Hant", "en-GB", "pt-BR"
	Title       string // human label, informational only
	Kind        Kind
	IsDefault   bool // container-declared default
	tag         Tag
}

// Preference expresses one entry of the user's language wish list.
type Preference struct {
	LanguageTag string
	PreferSDH   bool
	AllowForced bool
}

// AuditStep records one evaluated fallback stage for diagnostics.
type AuditStep struct {
	Stage      string // "preference" | "default" | "first" | "off"
	Requested  string // canonical requested tag or ""
	TrackID    string // track chosen at this stage, "" if none
	Score      int
	Confidence string
	Reason     string
	Candidates []string // considered track IDs in evaluation order
}

// Decision is the output of a selection run.
type Decision struct {
	TrackID  string // "" means subtitles off
	Off      bool
	Reason   string
	Steps    []AuditStep
	Warnings []string
}

// Selector holds parsed tracks and emits audit logs. It is safe for
// concurrent use because Select mutates no state.
type Selector struct {
	tracks []Track
	logger *slog.Logger
}

// New parses every track's language tag once and returns a Selector.
// A nil logger is replaced by slog.Default().
func New(tracks []Track, logger *slog.Logger) (*Selector, error) {
	if logger == nil {
		logger = slog.Default()
	}
	parsed := make([]Track, len(tracks))
	copy(parsed, tracks)
	for i := range parsed {
		tag, err := ParseTag(parsed[i].LanguageTag)
		if err != nil {
			return nil, fmt.Errorf("track %q has invalid language tag %q: %w",
				parsed[i].ID, parsed[i].LanguageTag, err)
		}
		parsed[i].tag = tag
	}
	return &Selector{tracks: parsed, logger: logger}, nil
}

// Select runs the deterministic fallback chain:
//
//  1. each user preference, in order, against eligible tracks;
//  2. the container-declared default track;
//  3. the first track in manifest order;
//  4. subtitles off.
//
// Every stage appends an AuditStep and emits a structured log entry.
func (s *Selector) Select(ctx context.Context, prefs []Preference, subtitlesOff bool) Decision {
	decision := Decision{}
	audit := func(step AuditStep) {
		decision.Steps = append(decision.Steps, step)
		attrs := []any{
			"stage", step.Stage,
			"requested", step.Requested,
			"track_id", step.TrackID,
			"score", step.Score,
			"confidence", step.Confidence,
			"reason", step.Reason,
		}
		if step.TrackID == "" && step.Stage != "off" {
			s.logger.WarnContext(ctx, "subtitle fallback stage missed", attrs...)
		} else {
			s.logger.InfoContext(ctx, "subtitle fallback stage", attrs...)
		}
	}

	if subtitlesOff {
		decision.Off = true
		decision.Reason = "subtitles disabled by user"
		audit(AuditStep{Stage: "off", Confidence: "off",
			Reason: decision.Reason})
		return decision
	}

	if len(s.tracks) == 0 {
		decision.Off = true
		decision.Reason = "no subtitle tracks available"
		decision.Warnings = append(decision.Warnings, decision.Reason)
		audit(AuditStep{Stage: "off", Confidence: "off",
			Reason: decision.Reason})
		return decision
	}

	for _, pref := range prefs {
		req, err := ParseTag(pref.LanguageTag)
		if err != nil {
			w := fmt.Sprintf("ignoring invalid preference tag %q: %v",
				pref.LanguageTag, err)
			decision.Warnings = append(decision.Warnings, w)
			s.logger.WarnContext(ctx, "invalid subtitle preference",
				"tag", pref.LanguageTag, "error", err)
			continue
		}
		if !req.HasLanguage() {
			continue
		}
		if id, step := s.chooseByLanguage(req, pref, audit); id != "" {
			decision.TrackID = id
			decision.Reason = step.Reason
			return decision
		}
	}

	if id, step := s.chooseDefault(audit); id != "" {
		decision.TrackID = id
		decision.Reason = step.Reason
		return decision
	}

	id, step := s.chooseFirst(audit)
	decision.TrackID = id
	decision.Off = id == ""
	decision.Reason = step.Reason
	return decision
}

// chooseByLanguage scores all eligible tracks for one preference and
// returns the best one, breaking ties deterministically.
func (s *Selector) chooseByLanguage(req Tag, pref Preference,
	audit func(AuditStep)) (string, AuditStep) {

	type scored struct {
		track Track
		score int
	}
	var ranked []scored
	var considered []string

	for _, tr := range s.tracks {
		if !kindAllowed(tr.Kind, pref) {
			continue
		}
		considered = append(considered, tr.ID)
		score := MatchScore(req, tr.tag)
		if score >= 0 {
			ranked = append(ranked, scored{tr, score})
		}
	}

	if len(ranked) == 0 {
		audit(AuditStep{
			Stage:      "preference",
			Requested:  req.String(),
			Confidence: "none",
			Score:      -1,
			Reason:     "no compatible language",
			Candidates: considered,
		})
		return "", AuditStep{}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		// Track kind precedes regional detail: never surprise the user
		// with an SDH/forced variant when a normal track of the same
		// language exists.
		ri, rj := kindRank(ranked[i].track.Kind, pref),
			kindRank(ranked[j].track.Kind, pref)
		if ri != rj {
			return ri < rj
		}
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].track.IsDefault != ranked[j].track.IsDefault {
			return ranked[i].track.IsDefault // default wins ties
		}
		return false // keep manifest order
	})

	best := ranked[0]
	reason := fmt.Sprintf("matched %s against %s (%s, score=%d)",
		req.String(), best.track.tag.String(),
		Confidence(best.score), best.score)
	step := AuditStep{
		Stage:      "preference",
		Requested:  req.String(),
		TrackID:    best.track.ID,
		Score:      best.score,
		Confidence: Confidence(best.score),
		Reason:     reason,
		Candidates: considered,
	}
	audit(step)
	return best.track.ID, step
}

func (s *Selector) chooseDefault(audit func(AuditStep)) (string, AuditStep) {
	for _, tr := range s.tracks {
		if tr.IsDefault && tr.Kind != KindCommentary && tr.Kind != KindForced {
			step := AuditStep{
				Stage:      "default",
				TrackID:    tr.ID,
				Confidence: "fallback",
				Reason:     "user preference unmet; using container default",
				Candidates: []string{tr.ID},
			}
			audit(step)
			return tr.ID, step
		}
	}
	audit(AuditStep{
		Stage:      "default",
		Confidence: "none",
		Reason:     "no default track declared",
	})
	return "", AuditStep{}
}

func (s *Selector) chooseFirst(audit func(AuditStep)) (string, AuditStep) {
	for _, tr := range s.tracks {
		if tr.Kind == KindCommentary || tr.Kind == KindForced {
			continue
		}
		step := AuditStep{
			Stage:      "first",
			TrackID:    tr.ID,
			Confidence: "fallback",
			Reason:     "last resort: first track in manifest order",
			Candidates: []string{tr.ID},
		}
		audit(step)
		return tr.ID, step
	}
	step := AuditStep{
		Stage:      "off",
		Confidence: "off",
		Reason:     "only commentary tracks exist; disabling subtitles",
	}
	audit(step)
	return "", step
}

func kindAllowed(k Kind, pref Preference) bool {
	switch k {
	case KindForced:
		return pref.AllowForced
	case KindCommentary:
		return false
	default:
		return true
	}
}

// kindRank orders same-score, same-language variants. SDH is promoted only
// when the user asked for it; forced and commentary always lose.
func kindRank(k Kind, pref Preference) int {
	switch k {
	case KindSDH:
		if pref.PreferSDH {
			return 0
		}
		return 2
	case KindNormal:
		return 1
	case KindForced:
		return 3
	default:
		return 4
	}
}
