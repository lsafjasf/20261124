package pgs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []Segment {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v (run `go run ./cmd/pgsgen`)", name, err)
	}
	segs, err := Parse(data)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return segs
}

func TestParseArrivalOrder(t *testing.T) {
	segs := loadFixture(t, "clean_two_epochs.sup")
	if len(segs) != 12 {
		t.Fatalf("got %d segments, want 12", len(segs))
	}
	// First display set must arrive palette, window, object, display, clear.
	want := []byte{TypePalette, TypeWindow, TypeObject, TypeDisplay, TypeClear}
	for i, w := range want {
		if segs[i].Type != w {
			t.Errorf("segment %d: got %s, want %s", i, TypeName(segs[i].Type), TypeName(w))
		}
	}
	if segs[0].PTS != 90000 || segs[4].PTS != 135000 {
		t.Errorf("unexpected PTS: seg0=%d seg4=%d", segs[0].PTS, segs[4].PTS)
	}
}

func TestCleanTwoEpochs(t *testing.T) {
	rep := Rebuild(loadFixture(t, "clean_two_epochs.sup"))
	if rep.FirstError != nil {
		t.Fatalf("unexpected error: %v", rep.FirstError)
	}
	if len(rep.Epochs) != 2 {
		t.Fatalf("got %d epochs, want 2", len(rep.Epochs))
	}
	wantSets := []DisplaySet{
		{Index: 0, StartPTS: 90000, EndPTS: 135000, SegmentFirst: 0, SegmentLast: 4},
		{Index: 1, StartPTS: 180000, EndPTS: 225000, SegmentFirst: 5, SegmentLast: 6},
		{Index: 2, StartPTS: 270000, EndPTS: 315000, SegmentFirst: 7, SegmentLast: 11},
	}
	if len(rep.Sets) != len(wantSets) {
		t.Fatalf("got %d display sets, want %d", len(rep.Sets), len(wantSets))
	}
	for i, w := range wantSets {
		if rep.Sets[i] != w {
			t.Errorf("set %d: got %+v, want %+v", i, rep.Sets[i], w)
		}
	}
	ep0, ep1 := rep.Epochs[0], rep.Epochs[1]
	if ep0.StartPTS != 90000 || ep0.EndPTS != 270000 {
		t.Errorf("epoch 0 span: %d..%d", ep0.StartPTS, ep0.EndPTS)
	}
	if ep1.StartPTS != 270000 || ep1.EndPTS != 315000 {
		t.Errorf("epoch 1 span: %d..%d", ep1.StartPTS, ep1.EndPTS)
	}
}

func TestNoCrossEpochPaletteReuse(t *testing.T) {
	rep := Rebuild(loadFixture(t, "clean_two_epochs.sup"))
	pal0, ok0 := rep.Epochs[0].Palettes[0]
	pal1, ok1 := rep.Epochs[1].Palettes[0]
	if !ok0 || !ok1 {
		t.Fatalf("both epochs must define palette 0: %v %v", ok0, ok1)
	}
	if pal0.Version != 1 || pal1.Version != 2 {
		t.Errorf("palette versions: epoch0=%d epoch1=%d, want 1 and 2",
			pal0.Version, pal1.Version)
	}
	if pal0 == pal1 {
		t.Error("epochs share a palette object: state leaked across epochs")
	}
}

func TestFirstErrorCrossEpochPalette(t *testing.T) {
	rep := Rebuild(loadFixture(t, "cross_epoch_palette.sup"))
	if rep.FirstError == nil {
		t.Fatal("expected an error, got none")
	}
	e := rep.FirstError
	if e.SegmentIndex != 5 || e.SegmentType != TypeDisplay || e.PTS != 180000 {
		t.Errorf("first error = segment %d type %s pts %d, want segment 5 display pts 180000",
			e.SegmentIndex, TypeName(e.SegmentType), e.PTS)
	}
	if !strings.Contains(e.Message, "cross-epoch palette reuse") {
		t.Errorf("unexpected message: %q", e.Message)
	}
	if len(rep.Epochs) != 2 || len(rep.Epochs[1].Palettes) != 0 {
		t.Errorf("epoch 1 must not inherit palettes: %+v", rep.Epochs)
	}
}

func TestFirstErrorResidualWindow(t *testing.T) {
	rep := Rebuild(loadFixture(t, "residual_window.sup"))
	if rep.FirstError == nil {
		t.Fatal("expected an error, got none")
	}
	e := rep.FirstError
	if e.SegmentIndex != 4 || e.SegmentType != TypeDisplay || e.PTS != 180000 {
		t.Errorf("first error = segment %d type %s pts %d, want segment 4 display pts 180000",
			e.SegmentIndex, TypeName(e.SegmentType), e.PTS)
	}
	if !strings.Contains(e.Message, "residual window") {
		t.Errorf("unexpected message: %q", e.Message)
	}
}

func TestFormatPTS(t *testing.T) {
	cases := map[uint32]string{
		0:      "00:00:00.000",
		90000:  "00:00:01.000",
		135000: "00:00:01.500",
		45045:  "00:00:00.500",
		90045:  "00:00:01.000", // 1.0005 s truncates to whole ms
	}
	for pts, want := range cases {
		if got := FormatPTS(pts); got != want {
			t.Errorf("FormatPTS(%d) = %s, want %s", pts, got, want)
		}
	}
}
