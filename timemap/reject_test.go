package timemap

import (
	"errors"
	"math/big"
	"testing"
)

func TestRejectNonMonotonicKeyframes(t *testing.T) {
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 0, 1),
		KF(1, 1, 1, 1),
		KF(1, 1, 2, 1), // same output time as previous keyframe
	})
	if _, err := Build(s); !errors.Is(err, errKeyframeOrder) {
		t.Fatalf("err = %v, want %v", err, errKeyframeOrder)
	}

	backwards := baseSpec24to30(48, []Keyframe{
		KF(1, 1, 0, 1),
		KF(0, 1, 1, 1),
	})
	if _, err := Build(backwards); !errors.Is(err, errKeyframeOrder) {
		t.Fatalf("err = %v, want %v", err, errKeyframeOrder)
	}
}

func TestRejectOutOfBoundsKeyframe(t *testing.T) {
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 0, 1),
		KF(2, 1, 5, 1), // clip is only 2s long
	})
	_, err := Build(s)
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("err = %v, want %v", err, ErrOutOfBounds)
	}
	var fe *FrameError
	if !errors.As(err, &fe) {
		t.Fatalf("error does not carry frame index: %v", err)
	}
}

func TestRejectNegativeSourceKeyframe(t *testing.T) {
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, -1, 2),
		KF(2, 1, 1, 1),
	})
	if _, err := Build(s); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("err = %v, want %v", err, ErrOutOfBounds)
	}
}

func TestRejectSubFrameLoop(t *testing.T) {
	// Forward, then a reverse turn whose excursion is half a source frame,
	// then forward again: the reverse run re-reads identical frames.
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 96,
		AudioRate:    Rate48000,
		OutputRate:   Rate30,
		OutputFrames: 90,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(1, 1, 1, 1),
			KF(2, 1, 47, 48), // source 1s -> 47/48 s (half a frame back)
			KF(3, 1, 2, 1),
		},
	}
	if _, err := Build(s); !errors.Is(err, ErrSubFrameLoop) {
		t.Fatalf("err = %v, want %v", err, ErrSubFrameLoop)
	}
}

func TestAllowOneFrameReverseRun(t *testing.T) {
	// Same shape, but the interior reverse run covers exactly one
	// source frame, which is a legal, observable rewind.
	s := Spec{
		SourceRate:   Rate24,
		SourceFrames: 96,
		AudioRate:    Rate48000,
		OutputRate:   Rate30,
		OutputFrames: 90,
		Keyframes: []Keyframe{
			KF(0, 1, 0, 1),
			KF(1, 1, 1, 1),
			KF(2, 1, 23, 24), // 1s - exactly one source frame
			KF(3, 1, 2, 1),
		},
	}
	tbl, err := Build(s)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tbl.Len() != 90 {
		t.Fatalf("Len = %d, want 90", tbl.Len())
	}
}

func TestRejectOutputBeyondKeyframes(t *testing.T) {
	s := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 0, 1),
		KF(1, 1, 1, 1), // profile ends at 1s but 60 output frames span 2s
	})
	_, err := Build(s)
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("err = %v, want %v", err, ErrOutOfBounds)
	}
}

func TestRejectInteriorFreeze(t *testing.T) {
	// Forward, zero-speed hold, forward again: the hold is a zero-excursion
	// interior turn with no observable position change.
	s := baseSpec24to30(72, []Keyframe{
		KF(0, 1, 0, 1),
		KF(1, 1, 1, 1),
		KF(2, 1, 1, 1),
		KF(3, 1, 2, 1),
	})
	if _, err := Build(s); !errors.Is(err, ErrSubFrameLoop) {
		t.Fatalf("err = %v, want %v", err, ErrSubFrameLoop)
	}
}

func TestAllowFreezeAtProfileEnds(t *testing.T) {
	start := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 1, 1),
		KF(1, 1, 1, 1),
	})
	start.OutputFrames = 30
	if _, err := Build(start); err != nil {
		t.Fatalf("freeze at start rejected: %v", err)
	}

	end := baseSpec24to30(48, []Keyframe{
		KF(0, 1, 0, 1),
		KF(1, 1, 1, 1),
		KF(2, 1, 1, 1),
	})
	if _, err := Build(end); err != nil {
		t.Fatalf("freeze at end rejected: %v", err)
	}
}

func TestRejectInvalidSpec(t *testing.T) {
	cases := map[string]Spec{
		"zero source rate": {SourceRate: big.NewRat(0, 1), OutputRate: Rate30, AudioRate: Rate48000, SourceFrames: 1, OutputFrames: 1, Keyframes: []Keyframe{KF(0, 1, 0, 1), KF(1, 1, 0, 1)}},
		"single keyframe":  baseSpec24to30(48, []Keyframe{KF(0, 1, 0, 1)}),
		"zero output frames": func() Spec {
			s := baseSpec24to30(48, []Keyframe{KF(0, 1, 0, 1), KF(1, 1, 1, 1)})
			s.OutputFrames = 0
			return s
		}(),
	}
	for name, s := range cases {
		if _, err := Build(s); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
	}
}
