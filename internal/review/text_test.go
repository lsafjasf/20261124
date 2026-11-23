package review

import (
	"math"
	"testing"
)

func TestNormalizeTextRemovesPresentationMarkup(t *testing.T) {
	got := normalizeText("<i>Ｈｅｌｌｏ</i>，  世界！&amp;")
	want := "hello世界"
	if got != want {
		t.Fatalf("normalizeText() = %q, want %q", got, want)
	}
}

func TestScoreTextUsesMoreThanExactEquality(t *testing.T) {
	evidence := scoreText("我们得马上离开", "<i>我们得马上离开了</i>！", 3)
	if evidence.Similarity <= 0.5 {
		t.Fatalf("similar variant scored too low: %+v", evidence)
	}
	if evidence.Similarity >= 1 {
		t.Fatalf("similar variant should not be treated as exact: %+v", evidence)
	}
	if evidence.EditSimilarity >= 1 || evidence.BigramDice >= 1 {
		t.Fatalf("fuzzy evidence must remain distinguishable: %+v", evidence)
	}
}

func TestScoreTextUnrelatedIsLow(t *testing.T) {
	evidence := scoreText("请立即疏散", "今日天气晴朗", 3)
	if evidence.Similarity >= 0.3 {
		t.Fatalf("unrelated text scored too high: %+v", evidence)
	}
}

func TestShortTextIsAdjustedForManualReview(t *testing.T) {
	evidence := scoreText("是", "是", 3)
	if !evidence.ShortTextAdjusted {
		t.Fatal("expected short text adjustment")
	}
	if math.Abs(evidence.EffectiveSimilarity-0.55) > 0.0001 {
		t.Fatalf("effective similarity = %v, want 0.55", evidence.EffectiveSimilarity)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"世界", "世 界", 1},
	}
	for _, test := range tests {
		if got := levenshtein([]rune(test.a), []rune(test.b)); got != test.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}
