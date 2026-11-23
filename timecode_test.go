package edl

import "testing"

func TestDropFrameAndNonDropFrameConversion(t *testing.T) {
	tests := []struct {
		text   string
		mode   TimecodeMode
		frames int64
	}{
		{"00:00:00;00", DF, 0},
		{"00:01:00;02", DF, 1800},
		{"01:00:00;00", DF, 107892},
		{"23:59:59;29", DF, 2589407},
		{"00:00:00:00", NDF, 0},
		{"00:01:00:00", NDF, 1800},
		{"01:00:00:00", NDF, 108000},
		{"-00:00:02;00", DF, -60},
	}
	for _, tt := range tests {
		tc, err := ParseTimecode(tt.text, Rate2997, tt.mode)
		if err != nil {
			t.Fatalf("ParseTimecode(%q): %v", tt.text, err)
		}
		if tc.Frames != tt.frames {
			t.Errorf("ParseTimecode(%q) = %d, want %d", tt.text, tc.Frames, tt.frames)
		}
		again, err := FromFrames(tt.frames, Rate2997, tt.mode)
		if err != nil {
			t.Fatalf("FromFrames(%d): %v", tt.frames, err)
		}
		if again.String() != tt.text {
			t.Errorf("FromFrames(%d) = %q, want %q", tt.frames, again.String(), tt.text)
		}
	}
}

func TestDropFrameRejectsDroppedLabels(t *testing.T) {
	for _, text := range []string{"00:01:00;00", "00:01:00;01", "00:11:00;00"} {
		if _, err := ParseTimecode(text, Rate2997, DF); err == nil {
			t.Errorf("ParseTimecode(%q) unexpectedly succeeded", text)
		}
	}
	if _, err := ParseTimecode("00:10:00;00", Rate2997, DF); err != nil {
		t.Errorf("10-minute exception label rejected: %v", err)
	}
}

func TestRateNormalization(t *testing.T) {
	for _, text := range []string{"29.97", "30000/1001"} {
		rate, err := ParseRate(text)
		if err != nil {
			t.Fatal(err)
		}
		if rate != Rate2997 || rate.String() != "30000/1001" {
			t.Errorf("ParseRate(%q) = %+v", text, rate)
		}
	}
}
