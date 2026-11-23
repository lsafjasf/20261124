package subtime

import "testing"

func TestConvertFrameOffersBothCandidates(t *testing.T) {
	point, err := ProjectFrame(Frame{Number: 1, Rate: Rate23976}, Rate25)
	if err != nil {
		t.Fatal(err)
	}
	if point.Position.String() != "1001/960" {
		t.Fatalf("converted position = %s", point.Position)
	}
	candidates, err := point.Candidates()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want floor and ceil", len(candidates))
	}
	if candidates[0].Frame != 1 || candidates[1].Frame != 2 {
		t.Fatalf("candidates = %+v", candidates)
	}
	if candidates[0].OffsetFrames.String() != "-41/960" {
		t.Fatalf("lower offset = %s", candidates[0].OffsetFrames)
	}
	if !candidates[0].IsNearest {
		t.Fatalf("lower candidate should be nearest: %+v", candidates)
	}
	if _, err := point.Snap(SnapNone); err == nil {
		t.Fatal("SnapNone must not silently round")
	}
}

func TestParseSnapPolicyIsCaseInsensitive(t *testing.T) {
	policy, err := ParseSnapPolicy(" Ceil ")
	if err != nil {
		t.Fatal(err)
	}
	if policy != SnapCeil {
		t.Fatalf("policy = %v", policy)
	}
}

func TestRoundTripRestoresOriginalFrames(t *testing.T) {
	frames := []int64{0, 1, 2, 23, 24, 25, 1799, 1800, 17981, 17982, 86399, 86400, 107891, 107892}
	paths := [][]Rate{
		{Rate25},
		{Rate2997DF},
		{Rate25, Rate2997DF},
		{Rate2997DF, Rate25},
		{Rate25, Rate2997DF, Rate25, Rate2997DF},
	}
	for _, frameNumber := range frames {
		frame := Frame{Number: frameNumber, Rate: Rate23976}
		for _, path := range paths {
			got, err := RoundTripN(frame, 3, path...)
			if err != nil {
				t.Fatalf("frame %d via %v: %v", frameNumber, path, err)
			}
			if got.Number != frameNumber || got.Rate != frame.Rate {
				t.Fatalf("frame %d via %v returned %+v", frameNumber, path, got)
			}
		}
	}
}

func TestRoundTripPropertyOverContinuousFrames(t *testing.T) {
	path := []Rate{Rate25, Rate2997DF, Rate25, Rate2997DF}
	for frameNumber := int64(0); frameNumber <= 20000; frameNumber++ {
		frame := Frame{Number: frameNumber, Rate: Rate23976}
		got, err := RoundTripN(frame, 2, path...)
		if err != nil {
			t.Fatalf("frame %d: %v", frameNumber, err)
		}
		if got.Number != frameNumber {
			t.Fatalf("frame %d recovered as %d", frameNumber, got.Number)
		}
	}
}
