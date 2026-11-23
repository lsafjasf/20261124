package fcpxml

import "testing"

func TestParseTimeIsExact(t *testing.T) {
	cases := map[string]string{
		"1001/24000s": "1001/24000",
		"1.5s":        "3/2",
		"0:00:01.5":   "3/2",
		"-1/25s":      "-1/25",
		"0s":          "0",
	}
	for input, want := range cases {
		got, err := ParseTime(input)
		if err != nil {
			t.Errorf("ParseTime(%q): %v", input, err)
			continue
		}
		if got.String() != want {
			t.Errorf("ParseTime(%q) = %s, want %s", input, got.String(), want)
		}
	}
}

func TestFramePositionUsesExactFrameDuration(t *testing.T) {
	frameDuration, err := ParseTime("1001/24000s")
	if err != nil {
		t.Fatal(err)
	}
	seconds, err := ParseTime("1001/24000s")
	if err != nil {
		t.Fatal(err)
	}
	position := framePosition(seconds.Rat(), frameDuration.Rat())
	if position.FrameExact != "1" || position.FrameRounded != 1 {
		t.Fatalf("position = %#v, want frame 1", position)
	}
}
