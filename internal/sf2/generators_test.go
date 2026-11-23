package sf2

import "testing"

func TestGeneratorNames(t *testing.T) {
	cases := map[uint16]string{
		GenStartAddrOffset:     "startAddrsOffset",
		GenStartLoopAddrOffset: "startloopAddrsOffset",
		GenKeyRange:            "keyRange",
		GenVelRange:            "velRange",
		GenKeyNum:              "keynum",
		GenVelocity:            "velocity",
		GenInstrumentID:        "instrument",
		GenSampleIDClassic:     "sampleID",
		GenSampleID:            "sampleID204",
		GenSampleModes:         "sampleModes204",
		GenOverridingRootKey:   "overridingRootKey",
	}
	if !isInstrumentIndexGen(GenInstrumentID) || !isInstrumentIndexGen(GenInstrumentID204) {
		t.Fatal("instrument index predicate must accept both enumerations")
	}
	if !isSampleIndexGen(GenSampleID) || !isSampleIndexGen(GenSampleIDClassic) {
		t.Fatal("sample index predicate must accept both enumerations")
	}
	for id, want := range cases {
		if got := GeneratorName(id); got != want {
			t.Errorf("generator %d: got name %q, want %q", id, got, want)
		}
	}
	if GeneratorName(200) != "unknown200" {
		t.Fatalf("unknown generator label wrong: %q", GeneratorName(200))
	}
	if IsKnownGenerator(200) {
		t.Fatal("id 200 must be treated as unknown")
	}
}
