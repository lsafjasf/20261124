// Package subselector implements a deterministic, pure-stdlib multilingual
// subtitle track selector.
//
// This file parses a pragmatic subset of BCP 47 language tags
// (language[-script][-region][-variant...] plus private-use/extension
// subtags) and scores how well a candidate track matches a requested tag.
package subselector

import (
	"errors"
	"strings"
)

// Tag is a parsed BCP 47 language tag.
type Tag struct {
	Raw      string   // original, normalized for comparison
	Language string   // ISO 639-1/639-3, lower case
	Script   string   // ISO 15924, title case; may be region-implied
	Region   string   // ISO 3166-1 alpha-2 / UN M.49, upper case
	Variants []string // lower case
	Private  []string // x-... subtags
}

// ErrInvalidTag is returned for malformed language tags.
var ErrInvalidTag = errors.New("invalid language tag")

// ParseTag parses a BCP 47 subset. Empty input yields an empty Tag with no
// error, representing "no language requested".
func ParseTag(raw string) (Tag, error) {
	raw = strings.TrimSpace(raw)
	tag := Tag{Raw: raw}
	if raw == "" {
		return tag, nil
	}

	parts := strings.Split(raw, "-")
	if len(parts) == 0 || !isAlpha(parts[0], 2, 3) {
		return Tag{}, ErrInvalidTag
	}
	tag.Language = strings.ToLower(parts[0])

	for i := 1; i < len(parts); i++ {
		p := parts[i]
		switch {
		case len(p) == 1: // extension / private-use singleton
			if p == "x" || p == "X" {
				tag.Private = append(tag.Private, lowerAll(parts[i+1:])...)
			}
			i = len(parts)
		case len(p) == 3 && isAlpha(p, 3, 3) && i == 1:
			return Tag{}, ErrInvalidTag // extlang subtags unsupported
		case len(p) == 4 && isAlpha(p, 4, 4):
			if tag.Script != "" {
				return Tag{}, ErrInvalidTag
			}
			tag.Script = titleCase(p)
		case len(p) == 2 && isAlpha(p, 2, 2) || len(p) == 3 && isDigit(p):
			if tag.Region != "" {
				return Tag{}, ErrInvalidTag
			}
			tag.Region = strings.ToUpper(p)
		case len(p) >= 4 && isAlnum(p):
			tag.Variants = append(tag.Variants, strings.ToLower(p))
		default:
			return Tag{}, ErrInvalidTag
		}
	}

	applyImpliedScript(&tag)
	tag.Raw = tag.String()
	return tag, nil
}

// String renders the canonical tag.
func (t Tag) String() string {
	var b strings.Builder
	b.WriteString(t.Language)
	if t.Script != "" {
		b.WriteByte('-')
		b.WriteString(t.Script)
	}
	if t.Region != "" {
		b.WriteByte('-')
		b.WriteString(t.Region)
	}
	for _, v := range t.Variants {
		b.WriteByte('-')
		b.WriteString(v)
	}
	return b.String()
}

// HasLanguage reports whether the tag carries any language.
func (t Tag) HasLanguage() bool { return t.Language != "" }

// MatchScore rates how well candidate satisfies a request. -1 means the
// languages are incompatible and the candidate must never be chosen.
// Higher is better; exact equality outranks every partial match.
func MatchScore(request, candidate Tag) int {
	if !request.HasLanguage() || !candidate.HasLanguage() ||
		request.Language != candidate.Language {
		return -1
	}
	if request.String() == candidate.String() {
		return 200
	}

	score := 100
	// Scripts: explicit mismatch (or implied-script mismatch) is fatal.
	if request.Script != "" && candidate.Script != "" {
		if request.Script != candidate.Script {
			return -1
		}
		score += 20
	} else if request.Script != "" || candidate.Script != "" {
		score -= 20 // only one side narrows on script
	}

	if request.Region != "" && candidate.Region != "" {
		if request.Region != candidate.Region {
			score -= 10 // same language, different region: below regionless
		} else {
			score += 20
		}
	}

	if len(request.Variants) > 0 && len(candidate.Variants) > 0 &&
		equalSet(request.Variants, candidate.Variants) {
		score += 5
	}
	return score
}

// Confidence converts a score into a human-readable match level.
func Confidence(score int) string {
	switch {
	case score < 0:
		return "none"
	case score >= 200:
		return "exact"
	case score >= 115:
		return "high"
	case score >= 90:
		return "partial"
	default:
		return "weak"
	}
}

// applyImpliedScript fills in a script that the region implies, so that
// zh-TW and zh-HK behave like Traditional Chinese and zh-CN like
// Simplified Chinese even when the script subtag is omitted.
func applyImpliedScript(t *Tag) {
	if t.Script != "" || t.Region == "" {
		return
	}
	switch t.Language {
	case "zh":
		switch t.Region {
		case "TW", "HK", "MO":
			t.Script = "Hant"
		case "CN", "SG":
			t.Script = "Hans"
		}
	}
}

func isAlpha(s string, min, max int) bool {
	if len(s) < min || len(s) > max {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

func isAlnum(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return false
		}
	}
	return len(s) > 0
}

func isDigit(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func titleCase(s string) string {
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			return false
		}
	}
	return true
}
