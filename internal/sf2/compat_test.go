package sf2

import "testing"

func TestROMSentinelIsWarning(t *testing.T) {
	b := validFile()
	b.igens[2] = genIndex(GenSampleIDClassic, 1000)
	rep := parseBuilder(t, b).Inspect()
	var sawWarning bool
	for _, d := range rep.Diagnostics {
		if d.Category == DiagMissingReference {
			if d.Severity != SevWarning {
				t.Fatalf("ROM sentinel must be a warning, got %+v", d)
			}
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Fatal("ROM sentinel diagnostic missing")
	}
	if len(rep.Resolved) != 1 {
		t.Fatalf("sentinel zone must be skipped, other zone still resolved: %d", len(rep.Resolved))
	}
}

func TestSF204Enumeration(t *testing.T) {
	b := validFile()
	// Switch both index generators to the 2.04 ids 49/53.
	b.pgens[0] = genIndex(GenInstrumentID204, 0)
	b.igens[2] = genIndex(GenSampleID, 0)
	b.igens[5] = genIndex(GenSampleID, 1)
	rep := parseBuilder(t, b).Inspect()
	if len(rep.Resolved) != 2 {
		t.Fatalf("2.04 enumeration should still resolve 2 zones: %+v", rep.Diagnostics)
	}
	for _, d := range rep.Diagnostics {
		if d.Severity == SevError {
			t.Fatalf("unexpected error for 2.04 ids: %+v", d)
		}
	}
}
