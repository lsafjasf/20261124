package sf2

import (
	"bytes"
	"strings"
	"testing"
)

func TestTruncatedRIFF(t *testing.T) {
	if _, err := Parse(bytes.NewReader([]byte("RIFF"))); err == nil {
		t.Fatal("expected error for truncated RIFF header")
	}
	b := validFile()
	raw := b.bytes()
	if _, err := Parse(bytes.NewReader(raw[:len(raw)-20])); err == nil {
		t.Fatal("expected error for truncated chunk payload")
	}
	raw[8] = 'X'
	if _, err := Parse(bytes.NewReader(raw)); err == nil ||
		!strings.Contains(err.Error(), "sfbk") {
		t.Fatalf("expected sfbk form type error, got %v", err)
	}
}

func TestMalformedChunkSize(t *testing.T) {
	b := validFile()
	raw := b.bytes()
	// Corrupt the phdr chunk length so it is no longer a multiple of 38.
	idx := bytes.Index(raw, []byte("phdr"))
	if idx < 0 {
		t.Fatal("phdr chunk not found")
	}
	raw[idx+4]++
	if _, err := Parse(bytes.NewReader(raw)); err == nil {
		t.Fatal("expected phdr alignment error")
	}
}

func TestGlobalInstrumentZone(t *testing.T) {
	b := validFile()
	// Add a global instrument zone (key/vel range, no sampleID) before the
	// two sample zones and shift every later bag index accordingly.
	b.ibags = append([]InstBag{{GenNdx: 2}}, b.ibags...)
	for i := 1; i < len(b.ibags); i++ {
		b.ibags[i].GenNdx += 2
	}
	b.insts[1].InstBagIdx = 3
	globalGens := []GenRecord{
		gen(GenKeyRange, int16(5|(120<<8))),
		gen(GenVelRange, int16(2|(90<<8))),
	}
	b.igens = append(globalGens, b.igens...)

	rep := parseBuilder(t, b).Inspect()
	if len(rep.Resolved) != 2 {
		t.Fatalf("expected 2 zones with global zone, got %d: %+v",
			len(rep.Resolved), rep.Diagnostics)
	}
	if rep.Resolved[0].KeyRange != (Range{0, 63}) {
		t.Fatalf("local key range must override global, got %+v", rep.Resolved[0].KeyRange)
	}
}

func TestIllegalGeneratorPlacement(t *testing.T) {
	b := validFile()
	b.pgens = append([]GenRecord{gen(GenSampleIDClassic, 0)}, b.pgens...)
	for i := range b.pbags {
		b.pbags[i].GenNdx += 1
	}
	rep := parseBuilder(t, b).Inspect()
	found := false
	for _, d := range rep.Diagnostics {
		if d.Category == DiagStructure && strings.Contains(d.Message, "illegal in preset zone") {
			found = true
		}
	}
	if !found {
		t.Fatalf("illegal preset-zone sampleID not reported: %+v", rep.Diagnostics)
	}
}
