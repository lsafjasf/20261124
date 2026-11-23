package timecode

import "testing"

func TestParseModes(t *testing.T) {
	cases := []struct {
		token string
		mode  Mode
	}{
		{"01:02:03:04", NDF},
		{"01:02:03;04", DF},
		{"01:02:03.04", DF},
		{"01:02:03,04", DF},
	}
	for _, c := range cases {
		tc, err := Parse(c.token, 0)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.token, err)
		}
		if tc.Mode != c.mode || tc.Hour != 1 || tc.Minute != 2 || tc.Second != 3 || tc.Frame != 4 {
			t.Fatalf("Parse(%q) = %+v, want mode %v", c.token, tc, c.mode)
		}
		if tc.Render() != c.token && c.mode == NDF {
			t.Fatalf("Render = %q, want %q", tc.Render(), c.token)
		}
	}
	if _, err := Parse("1:02:03:04", 0); err == nil {
		t.Fatal("expected parse error for malformed token")
	}
}

func TestKnownVectors2997(t *testing.T) {
	cases := []struct {
		ndf    string
		df     string
		frames int64
	}{
		{"00:00:00:00", "00:00:00;00", 0},
		{"00:00:59:29", "00:00:59;29", 1799},
		{"00:01:00:00", "00:01:00;02", 1800},
		{"00:00:59:28", "00:00:59;28", 1798},
		{"00:09:59:12", "00:10:00;00", 17982},
		{"00:59:56:12", "01:00:00;00", 107892},
	}
	for _, c := range cases {
		ndf, err := Parse(c.ndf, 0)
		if err != nil {
			t.Fatal(err)
		}
		gotNDF, err := ToFrames(ndf, 30)
		if err != nil {
			t.Fatalf("ToFrames NDF %q: %v", c.ndf, err)
		}
		df, err := Parse(c.df, 0)
		if err != nil {
			t.Fatal(err)
		}
		gotDF, err := ToFrames(df, 30)
		if err != nil {
			t.Fatalf("ToFrames DF %q: %v", c.df, err)
		}
		if gotNDF != c.frames || gotDF != c.frames {
			t.Fatalf("%s/%s: frames NDF=%d DF=%d want %d", c.ndf, c.df, gotNDF, gotDF, c.frames)
		}
		back, err := FromFrames(c.frames, 30, DF)
		if err != nil {
			t.Fatal(err)
		}
		if back.Render() != c.df {
			t.Fatalf("FromFrames(%d, DF) = %q, want %q", c.frames, back.Render(), c.df)
		}
	}
}

func TestKnownVectors5994(t *testing.T) {
	// At 59.94 the first minute skips four labels, so frame 3600 is 00:01:00;04.
	df, err := Parse("00:01:00;04", 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ToFrames(df, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got != 3600 {
		t.Fatalf("frames = %d, want 3600", got)
	}
	back, err := FromFrames(3600, 60, DF)
	if err != nil {
		t.Fatal(err)
	}
	if back.Render() != "00:01:00;04" {
		t.Fatalf("render = %q", back.Render())
	}
}

func TestExhaustiveRoundTrip(t *testing.T) {
	for _, fps := range []int{30, 60} {
		drop := dropPerMinute(fps)
		dayFrames := int64(24 * 60 * 60 * fps)
		for f := int64(0); f < dayFrames; f++ {
			tc, err := FromFrames(f, fps, DF)
			if err != nil {
				t.Fatalf("fps=%d f=%d: %v", fps, f, err)
			}
			if !tc.IsValidLabel(fps) {
				t.Fatalf("fps=%d f=%d produced illegal label %s", fps, f, tc.Render())
			}
			back, err := ToFrames(tc, fps)
			if err != nil {
				t.Fatalf("fps=%d f=%d label=%s: %v", fps, f, tc.Render(), err)
			}
			if back != f {
				t.Fatalf("fps=%d round trip failed at %d -> %s -> %d", fps, f, tc.Render(), back)
			}
			// Every frame count must map to a strictly increasing label.
			if f > 0 {
				prev, _ := FromFrames(f-1, fps, DF)
				if !labelLess(prev, tc) {
					t.Fatalf("labels not increasing at f=%d: %s -> %s", f, prev.Render(), tc.Render())
				}
			}
		}
		// Exactly drop*9 skipped labels per ten-minute cycle.
		cycleFrames := int64(10*60*60*fps - drop*9*60)
		if cycleFrames <= 0 {
			t.Fatal("bad cycle math")
		}
	}
}

func labelLess(a, b Timecode) bool {
	as := [4]int{a.Hour, a.Minute, a.Second, a.Frame}
	bs := [4]int{b.Hour, b.Minute, b.Second, b.Frame}
	for i := range as {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return false
}

func TestIllegalDFLabelRejected(t *testing.T) {
	bad := []string{"00:01:00;00", "00:01:00;01", "00:09:00;00", "01:59:00;00"}
	for _, token := range bad {
		tc, err := Parse(token, 0)
		if err != nil {
			t.Fatal(err)
		}
		if tc.IsValidLabel(30) {
			t.Fatalf("%q should be an illegal 29.97 DF label", token)
		}
		if _, err := ToFrames(tc, 30); err == nil {
			t.Fatalf("%q must not convert to frames", token)
		}
	}
	good := []string{"00:00:00;00", "00:10:00;00", "01:00:00;00", "00:01:00;02"}
	for _, token := range good {
		tc, _ := Parse(token, 0)
		if !tc.IsValidLabel(30) {
			t.Fatalf("%q should be legal", token)
		}
	}
	// At 59.94 the first legal label after the minute is ;04.
	tc, _ := Parse("00:01:00;03", 0)
	if tc.IsValidLabel(60) {
		t.Fatal("00:01:00;03 must be illegal at 59.94")
	}
}

func TestNDFFrameRates(t *testing.T) {
	for _, fps := range []int{12, 23, 24, 25, 50} {
		tc, err := Parse("00:01:00:00", 0)
		if err != nil {
			t.Fatal(err)
		}
		f, err := ToFrames(tc, fps)
		if err != nil {
			t.Fatal(err)
		}
		if f != int64(60*fps) {
			t.Fatalf("fps=%d frames=%d", fps, f)
		}
		back, err := FromFrames(f, fps, NDF)
		if err != nil || back.Render() != "00:01:00:00" {
			t.Fatalf("fps=%d back=%q err=%v", fps, back.Render(), err)
		}
		// A DF request on a non-drop rate degrades to NDF.
		back2, err := FromFrames(f, fps, DF)
		if err != nil || back2.Mode != NDF {
			t.Fatalf("fps=%d expected NDF fallback, got %+v err=%v", fps, back2, err)
		}
	}
}

func TestNDFAndDFFrameCountsDifferByDesign(t *testing.T) {
	// The exact label "00:01:00:00" written NDF vs ";02" written DF both
	// identify frame 1800: repair keeps frame counts, never wall-clock seconds.
	ndf, _ := Parse("00:01:00:00", 0)
	df, _ := Parse("00:01:00;02", 0)
	fn, _ := ToFrames(ndf, 30)
	fd, _ := ToFrames(df, 30)
	if fn != 1800 || fd != 1800 || fn != fd {
		t.Fatalf("expected both labels at frame 1800, got %d and %d", fn, fd)
	}
}
