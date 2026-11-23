package review

import (
	"html"
	"math"
	"regexp"
	"strings"
	"unicode"
)

var (
	htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)
	assTagPattern  = regexp.MustCompile(`\{[^}]*\}`)
)

// normalizeText removes presentation markup, width differences and punctuation
// without attempting any language or meaning inference.
func normalizeText(text string) string {
	text = html.UnescapeString(text)
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = assTagPattern.ReplaceAllString(text, "")
	var b strings.Builder
	for _, r := range text {
		r = foldFullwidth(r)
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func foldFullwidth(r rune) rune {
	switch {
	case r == '\u3000':
		return ' '
	case r >= '\uff01' && r <= '\uff5e':
		return r - 0xfee0
	default:
		return r
	}
}

func scoreText(forced, full string, shortTextRunes int) TextEvidence {
	normalizedForced := normalizeText(forced)
	normalizedFull := normalizeText(full)
	a := []rune(normalizedForced)
	b := []rune(normalizedFull)

	evidence := TextEvidence{
		ForcedText:       forced,
		FullText:         full,
		NormalizedForced: normalizedForced,
		NormalizedFull:   normalizedFull,
	}
	if len(a) == 0 || len(b) == 0 {
		return evidence
	}

	shortest := len(a)
	if len(b) < shortest {
		shortest = len(b)
	}
	longest := len(a)
	if len(b) > longest {
		longest = len(b)
	}
	evidence.ShortestTextRunes = shortest

	distance := levenshtein(a, b)
	evidence.EditSimilarity = clamp01(1 - float64(distance)/float64(longest))
	evidence.ContainmentScore = clamp01(1 - float64(distance)/float64(shortest))
	evidence.BigramDice = bigramDice(a, b)
	evidence.Similarity = clamp01(
		0.50*evidence.EditSimilarity +
			0.30*evidence.ContainmentScore +
			0.20*evidence.BigramDice,
	)

	if shortTextRunes > 0 && shortest < shortTextRunes {
		evidence.ShortTextAdjusted = true
		evidence.EffectiveSimilarity = math.Min(evidence.Similarity, 0.55)
	} else {
		evidence.EffectiveSimilarity = evidence.Similarity
	}
	return evidence
}

func levenshtein(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	if len(b) == 0 {
		return len(a)
	}
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min3(
				current[j-1]+1,
				previous[j]+1,
				previous[j-1]+cost,
			)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func bigramDice(a, b []rune) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) < 2 || len(b) < 2 {
		if len(a) == len(b) && len(a) == 1 && a[0] == b[0] {
			return 1
		}
		return 0
	}
	counts := make(map[[2]rune]int, len(a)-1)
	for i := 0; i < len(a)-1; i++ {
		counts[[2]rune{a[i], a[i+1]}]++
	}
	common := 0
	for i := 0; i < len(b)-1; i++ {
		key := [2]rune{b[i], b[i+1]}
		if counts[key] > 0 {
			counts[key]--
			common++
		}
	}
	return 2 * float64(common) / float64((len(a)-1)+(len(b)-1))
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
