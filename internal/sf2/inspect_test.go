package sf2

import (
	"bytes"
	"testing"
)

func validFile() *sf2Builder {
	// Two samples: s0 frames [0,100) loop [10,40); s1 frames [100,200)
	// loop [110,140). One instrument with two sample zones and one preset.
	b := &sf2Builder{
		presets: []PresetHeader{
			{Name: "Piano", Preset: 0, Bank: 0, PresetBagIdx: 0},
			{Name: "EOP", PresetBagIdx: 1}, // points at terminal bag
		},
		pbags: []PresetBag{
			{GenNdx: 0}, // zone: instrumentID
			{GenNdx: 1}, // terminal bag: endOper; last row = gen upper bound
		},
		pgens: []GenRecord{
			genIndex(GenInstrumentID, 0),
			gen(60, 0), // endOper sentinel
		},
		insts: []InstHeader{
			{Name: "PianoInst", InstBagIdx: 0},
			{Name: "EOI", InstBagIdx: 2}, // points at terminal bag
		},
		ibags: []InstBag{
			{GenNdx: 0}, // zone 0: 3 gens
			{GenNdx: 3}, // zone 1: 3 gens
			{GenNdx: 6}, // terminal bag: endOper; last row = gen upper bound
		},
		igens: []GenRecord{
			gen(GenKeyRange, int16(0|(63<<8))),
			gen(GenVelRange, int16(1|(100<<8))),
			genIndex(GenSampleIDClassic, 0),
			gen(GenKeyRange, int16(64|(127<<8))),
			gen(GenVelRange, int16(1|(127<<8))),
			genIndex(GenSampleIDClassic, 1),
			gen(60, 0), // endOper sentinel
		},
		samples: []SampleHeader{
			{Name: "s0", Start: 0, End: 100, LoopStart: 10, LoopEnd: 40,
				Rate: 44100, RootKey: 60, Link: SampleLink{Type: SampleTypeMono}},
			{Name: "s1", Start: 100, End: 200, LoopStart: 110, LoopEnd: 140,
				Rate: 44100, RootKey: 72, Link: SampleLink{Type: SampleTypeMono}},
			{Name: "EOS", Link: SampleLink{Type: SampleTypeMono}},
		},
		smpl: make([]byte, 400), // 200 frames * 2 bytes
	}
	return b
}

func parseBuilder(t *testing.T, b *sf2Builder) *File {
	t.Helper()
	f, err := Parse(bytes.NewReader(b.bytes()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func TestValidExpansion(t *testing.T) {
	rep := parseBuilder(t, validFile()).Inspect()
	if len(rep.Resolved) != 2 {
		t.Fatalf("expected 2 resolved sample zones, got %d", len(rep.Resolved))
	}
	r0 := rep.Resolved[0]
	if r0.SampleName != "s0" || r0.InstrumentName != "PianoInst" || r0.PresetName != "Piano" {
		t.Fatalf("unexpected chain: %+v", r0)
	}
	if r0.KeyRange != (Range{0, 63}) || r0.VelRange != (Range{1, 100}) {
		t.Fatalf("ranges wrong: %+v", r0)
	}
	if r0.RootKey != 60 || r0.Loop.DeclaredStart != 10 || r0.Loop.DeclaredEnd != 40 {
		t.Fatalf("root/loop wrong: %+v", r0)
	}
	if diags(rep, SevError) != 0 {
		t.Fatalf("unexpected errors: %+v", rep.Diagnostics)
	}
}

func TestMissingInstrumentReference(t *testing.T) {
	b := validFile()
	b.pgens[0] = genIndex(GenInstrumentID, 99)
	rep := parseBuilder(t, b).Inspect()
	if !hasDiag(rep, DiagMissingReference, "phdr[0].pgen zone 1") {
		t.Fatalf("missing instrument ref not located: %+v", rep.Diagnostics)
	}
}

func TestMissingSampleReference(t *testing.T) {
	b := validFile()
	b.igens[2] = genIndex(GenSampleIDClassic, 42)
	rep := parseBuilder(t, b).Inspect()
	if !hasDiag(rep, DiagMissingReference, "inst[0].igen zone 1") {
		t.Fatalf("missing sample ref not located: %+v", rep.Diagnostics)
	}
}

func TestOverlappingLoopAcrossSamples(t *testing.T) {
	b := validFile()
	// s1 loop reaches back into s0's loop region.
	b.samples[1].LoopStart = 30
	b.samples[1].LoopEnd = 50
	rep := parseBuilder(t, b).Inspect()
	if !hasDiag(rep, DiagOverlappingLoop, "shdr[0]") {
		t.Fatalf("cross-sample loop overlap not located: %+v", rep.Diagnostics)
	}
}

func TestLoopOutsideDataRange(t *testing.T) {
	b := validFile()
	b.samples[0].LoopEnd = 140
	rep := parseBuilder(t, b).Inspect()
	if !hasDiag(rep, DiagOverlappingLoop, "shdr[0]") {
		t.Fatalf("out-of-range loop not located: %+v", rep.Diagnostics)
	}
}

func TestIllegalRootKeyInShdr(t *testing.T) {
	b := validFile()
	b.samples[0].RootKey = 128
	rep := parseBuilder(t, b).Inspect()
	if !hasDiag(rep, DiagIllegalRootKey, "shdr[0]") {
		t.Fatalf("illegal shdr root key not located: %+v", rep.Diagnostics)
	}
}

func TestIllegalRootKeyFromGenerator(t *testing.T) {
	b := validFile()
	b.igens = append(b.igens[:2],
		append([]GenRecord{gen(GenOverridingRootKey, 200)}, b.igens[2:]...)...)
	for i := range b.ibags {
		b.ibags[i].GenNdx += 1
	}
	rep := parseBuilder(t, b).Inspect()
	if !hasDiag(rep, DiagIllegalRootKey, "gen(overridingRootKey)") {
		t.Fatalf("illegal generator root key not located: %+v", rep.Diagnostics)
	}
}

func TestUnknownGeneratorPreserved(t *testing.T) {
	b := validFile()
	b.igens[2] = gen(200, -1234) // undefined id
	b.igens = append(b.igens[:3],
		append([]GenRecord{genIndex(GenSampleIDClassic, 0)}, b.igens[3:]...)...)
	for i := 1; i < len(b.ibags); i++ {
		b.ibags[i].GenNdx += 1
	}
	rep := parseBuilder(t, b).Inspect()
	var found bool
	for _, r := range rep.Resolved {
		for _, g := range r.Generators {
			if g.ID == 200 && g.Amount == -1234 && !g.Known && g.Name == "unknown200" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("unknown generator not preserved: %+v", rep.Resolved)
	}
}

func TestLoopGeneratorOffsets(t *testing.T) {
	b := validFile()
	// Shift s0 effective loop by +2 (fine) and +32768 (one coarse unit).
	b.igens = append(b.igens[:2],
		append([]GenRecord{
			gen(GenStartLoopAddrOffset, 2),
			gen(GenStartLoopAddrCoarseOffset, 1),
		}, b.igens[2:]...)...)
	for i := 1; i < len(b.ibags); i++ {
		b.ibags[i].GenNdx += 2
	}
	rep := parseBuilder(t, b).Inspect()
	if rep.Resolved[0].Loop.EffectiveStart != 10+2+32768 {
		t.Fatalf("coarse/fine loop offset not applied: %+v", rep.Resolved[0].Loop)
	}
}

func diags(rep *Report, sev string) int {
	n := 0
	for _, d := range rep.Diagnostics {
		if d.Severity == sev {
			n++
		}
	}
	return n
}

func hasDiag(rep *Report, cat, loc string) bool {
	for _, d := range rep.Diagnostics {
		if d.Category == cat && d.Location == loc {
			return true
		}
	}
	return false
}
