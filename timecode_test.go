package subtime

import "testing"

func TestParseRateUsesExactNTSCFractions(t *testing.T) {
	rate, err := ParseRate("23.976")
	if err != nil {
		t.Fatal(err)
	}
	if rate.Num != 24000 || rate.Den != 1001 || rate.DropFrame {
		t.Fatalf("23.976 parsed as %+v", rate)
	}

	rate, err = ParseRate("29.97df")
	if err != nil {
		t.Fatal(err)
	}
	if rate.Num != 30000 || rate.Den != 1001 || !rate.DropFrame {
		t.Fatalf("29.97df parsed as %+v", rate)
	}

	rate, err = ParseRate("29.97ndf")
	if err != nil {
		t.Fatal(err)
	}
	if rate.DropFrame {
		t.Fatalf("29.97ndf parsed as drop-frame")
	}

	if _, err := ParseRate("23.976df"); err == nil {
		t.Fatal("23.976df should be invalid")
	}
}

func TestDropFrameHourBoundary(t *testing.T) {
	cases := []struct {
		frame int64
		text  string
	}{
		{0, "00:00:00;00"},
		{1798, "00:00:59;28"},
		{1799, "00:00:59;29"},
		{1800, "00:01:00;02"},
		{17981, "00:09:59;29"},
		{17982, "00:10:00;00"},
		{107891, "00:59:59;29"},
		{107892, "01:00:00;00"},
		{215783, "01:59:59;29"},
		{215784, "02:00:00;00"},
	}
	for _, tc := range cases {
		frame := Frame{Number: tc.frame, Rate: Rate2997DF}
		got, err := FormatTimecode(frame)
		if err != nil {
			t.Fatalf("format %d: %v", tc.frame, err)
		}
		if got != tc.text {
			t.Errorf("format %d = %q, want %q", tc.frame, got, tc.text)
		}
		parsed, err := ParseTimecode(tc.text, Rate2997DF)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.text, err)
		}
		if parsed.Number != tc.frame {
			t.Errorf("parse %q = %d, want %d", tc.text, parsed.Number, tc.frame)
		}
	}
}

func TestDropFrameRejectsSkippedLabels(t *testing.T) {
	for _, text := range []string{"00:01:00;00", "00:01:00;01", "00:59:00;01"} {
		if _, err := ParseTimecode(text, Rate2997DF); err == nil {
			t.Errorf("ParseTimecode(%q) accepted a dropped label", text)
		}
	}
	if _, err := ParseTimecode("00:10:00;00", Rate2997DF); err != nil {
		t.Fatalf("tenth minute should keep frame 00: %v", err)
	}
}

func TestParseClockIsExact(t *testing.T) {
	clock, err := ParseClock("01:02:03,456")
	if err != nil {
		t.Fatal(err)
	}
	want, err := ParseRational("3723.456")
	if err != nil {
		t.Fatal(err)
	}
	if !clock.Equal(want) {
		t.Fatalf("clock = %s, want %s", clock, want)
	}
}
