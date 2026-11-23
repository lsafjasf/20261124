package timebase

import (
	"encoding/json"
	"math/big"
	"testing"
)

func mustRate(t *testing.T, s string) Rate {
	t.Helper()
	r, err := ParseRate(s)
	if err != nil {
		t.Fatalf("ParseRate(%q): %v", s, err)
	}
	return r
}

func TestParseRateNormalizesNTSC(t *testing.T) {
	for _, s := range []string{"23.976", "23.98", "24000/1001"} {
		if got := mustRate(t, s); got.Num != 24000 || got.Den != 1001 {
			t.Fatalf("ParseRate(%q) = %s, want 24000/1001", s, got)
		}
	}
}

func TestCountInAndQuantize(t *testing.T) {
	fps := mustRate(t, "24000/1001")
	ms := Rate{Num: 1000, Den: 1}

	q := Quantity{Count: 24, Rate: fps}
	exact := q.CountIn(ms)
	if got := exact.RatString(); got != "1001" {
		t.Fatalf("24 frames in ms = %s, want 1001", got)
	}
	for _, tc := range []struct {
		in   string
		mode Rounding
		want int64
	}{
		{"1/2", Floor, 0},
		{"1/2", Ceil, 1},
		{"1/2", Nearest, 1},
		{"-1/2", Floor, -1},
		{"-1/2", Ceil, 0},
		{"-1/2", Nearest, -1},
		{"3/2", Nearest, 2},
	} {
		v, ok := new(big.Rat).SetString(tc.in)
		if !ok {
			t.Fatalf("bad test rational %q", tc.in)
		}
		got, err := Quantize(v, tc.mode)
		if err != nil {
			t.Fatalf("Quantize(%s,%s): %v", tc.in, tc.mode, err)
		}
		if got != tc.want {
			t.Fatalf("Quantize(%s,%s) = %d, want %d", tc.in, tc.mode, got, tc.want)
		}
	}
}

func TestLegacyRateOffByOnePartIn1001(t *testing.T) {
	actual := mustRate(t, "24000/1001")
	nominal := mustRate(t, "24")
	pts := Rate{Num: 90000, Den: 1}

	frame := int64(11988)
	good, err := ConvertStep("actual", Quantity{Count: frame, Rate: actual}, pts, Nearest)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := ConvertStep("nominal", Quantity{Count: frame, Rate: nominal}, pts, Floor)
	if err != nil {
		t.Fatal(err)
	}
	if good.Output.Count != 44999955 {
		t.Fatalf("actual PTS = %d, want 44999955", good.Output.Count)
	}
	if bad.Output.Count != 44955000 {
		t.Fatalf("nominal PTS = %d, want 44955000", bad.Output.Count)
	}
	diff := new(big.Rat).Sub(good.Output.Seconds(), bad.Output.Seconds())
	if got := diff.FloatString(6); got != "0.499500" {
		t.Fatalf("end drift = %s s, want 0.499500", got)
	}
}

func TestRateJSONForms(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{`"24000/1001"`, "24000/1001"},
		{`90000`, "90000"},
		{`{"num":24,"den":1}`, "24"},
		{`"23.976"`, "24000/1001"},
	} {
		var r Rate
		if err := json.Unmarshal([]byte(tc.in), &r); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.in, err)
		}
		if got := r.String(); got != tc.want {
			t.Fatalf("unmarshal %s = %s, want %s", tc.in, got, tc.want)
		}
	}
}
