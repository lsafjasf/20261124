package subtitle

import (
	"slices"
	"testing"
)

func TestCellWidthCJK(t *testing.T) {
	if got := CellWidth("你好ab"); got != 6 {
		t.Fatalf("expected width 6, got %d", got)
	}
}

func TestWrapLineWordBreak(t *testing.T) {
	lines := WrapLine("alpha beta gamma", 11)
	want := []string{"alpha beta", "gamma"}
	if !slices.Equal(lines, want) {
		t.Fatalf("expected %v, got %v", want, lines)
	}
}

func TestWrapLineHardBreak(t *testing.T) {
	lines := WrapLine("abcdefghij", 4)
	want := []string{"abcd", "efgh", "ij"}
	if !slices.Equal(lines, want) {
		t.Fatalf("expected %v, got %v", want, lines)
	}
}

func TestFitCueOverflow(t *testing.T) {
	cue := Cue{Lines: []string{"one", "two", "three"}, WidthCells: 5}
	fit := FitCue(cue, ReadableSpace{WidthCells: 20, HeightLines: 2})
	if fit.Fits || fit.Overflow != 1 || fit.Lines != 3 {
		t.Fatalf("unexpected fit: %+v", fit)
	}
}

func TestFitCueWidthOverflow(t *testing.T) {
	cue := Cue{Lines: []string{"abcdefghij"}, WidthCells: 10}
	fit := FitCue(cue, ReadableSpace{WidthCells: 4, HeightLines: 10})
	if fit.Fits || fit.WidthCells <= 4 {
		t.Fatalf("expected width overflow, got %+v", fit)
	}
}

func TestFitCueFits(t *testing.T) {
	cue := Cue{Lines: []string{"hello there"}, WidthCells: 11}
	fit := FitCue(cue, ReadableSpace{WidthCells: 40, HeightLines: 2})
	if !fit.Fits {
		t.Fatalf("expected cue to fit, got %+v", fit)
	}
}
