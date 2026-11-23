package sf2

import "fmt"

type diagFn func(cat, sev, loc, format string, args ...any)

// validateTables checks index consistency inside and between the seven
// tables. Problems that make a reference unresolvable are reported as
// missing-reference; malformed table shapes are reported as structure.
func (f *File) validateTables(pBags, iBags []bagRecord, add diagFn) {
	f.validateBagChains("pbag", pBags, len(f.PGens), add)
	f.validateBagChains("ibag", iBags, len(f.IGens), add)
	f.validateTerminalRecords(add)

	for i, g := range f.PGens {
		if isSampleIndexGen(g.ID) {
			add(DiagStructure, SevError, fmt.Sprintf("pgen[%d]", i),
				"sampleID generator is illegal in preset zone")
		}
	}
	for i, g := range f.IGens {
		if isInstrumentIndexGen(g.ID) {
			add(DiagStructure, SevError, fmt.Sprintf("igen[%d]", i),
				"instrumentID generator is illegal in instrument zone")
		}
	}
}

func (f *File) validateBagChains(name string, bags []bagRecord, genLen int, add diagFn) {
	for i, b := range bags {
		if int(b.GenNdx) > genLen {
			add(DiagStructure, SevError, fmt.Sprintf("%s[%d]", name, i),
				"generator index %d exceeds %s length %d", b.GenNdx, name, genLen)
		}
		if i > 0 && b.GenNdx < bags[i-1].GenNdx {
			add(DiagStructure, SevError, fmt.Sprintf("%s[%d]", name, i),
				"generator index %d is lower than previous bag's %d",
				b.GenNdx, bags[i-1].GenNdx)
		}
	}
}

func (f *File) validateTerminalRecords(add diagFn) {
	if len(f.Presets) == 0 {
		add(DiagStructure, SevError, "phdr", "phdr chunk is missing terminal EOP record")
	} else if f.Presets[len(f.Presets)-1].Name != "EOP" {
		add(DiagStructure, SevWarning, "phdr",
			"terminal preset is %q, expected EOP", f.Presets[len(f.Presets)-1].Name)
	}
	if len(f.Insts) == 0 {
		add(DiagStructure, SevError, "inst", "inst chunk is missing terminal EOI record")
	} else if f.Insts[len(f.Insts)-1].Name != "EOI" {
		add(DiagStructure, SevWarning, "inst",
			"terminal instrument is %q, expected EOI", f.Insts[len(f.Insts)-1].Name)
	}
	if len(f.Samples) == 0 {
		add(DiagStructure, SevError, "shdr", "shdr chunk is missing terminal EOS sample")
	} else if f.Samples[len(f.Samples)-1].Name != "EOS" {
		add(DiagStructure, SevWarning, "shdr",
			"terminal sample is %q, expected EOS", f.Samples[len(f.Samples)-1].Name)
	}
}

// checkSamples validates every non-terminal shdr record on its own and then
// checks loop regions of different samples for overlaps inside smpl space.
func (f *File) checkSamples(rep *Report, add diagFn, sampleCount int) {
	type loopRegion struct {
		idx        int
		start, end uint32
	}
	var loops []loopRegion

	for si := 0; si < sampleCount; si++ {
		sh := f.Samples[si]
		loc := fmt.Sprintf("shdr[%d]", si)
		if sh.RootKey != 255 && sh.RootKey > 127 {
			add(DiagIllegalRootKey, SevError, loc,
				"sample %q originalPitch %d is outside 0..127 (255 = unspecified)",
				sh.Name, sh.RootKey)
		}
		if f.SmplFrames > 0 && (sh.Start > f.SmplFrames || sh.End > f.SmplFrames) {
			add(DiagMissingReference, SevError, loc,
				"sample %q data range [%d,%d) exceeds smpl length %d frames",
				sh.Name, sh.Start, sh.End, f.SmplFrames)
		}
		if sh.End < sh.Start {
			add(DiagStructure, SevError, loc,
				"sample %q end %d precedes start %d", sh.Name, sh.End, sh.Start)
		}
		if sh.LoopEnd < sh.LoopStart {
			add(DiagOverlappingLoop, SevError, loc,
				"sample %q loop end %d precedes loop start %d",
				sh.Name, sh.LoopEnd, sh.LoopStart)
		}
		if sh.LoopStart < sh.Start || sh.LoopEnd > sh.End {
			add(DiagOverlappingLoop, SevError, loc,
				"sample %q loop [%d,%d) falls outside data range [%d,%d)",
				sh.Name, sh.LoopStart, sh.LoopEnd, sh.Start, sh.End)
		}
		if sh.Rate == 0 {
			add(DiagStructure, SevWarning, loc, "sample %q has sample rate 0", sh.Name)
		}
		if (sh.Link.Type &^ SampleTypeROM) != SampleTypeMono {
			if int(sh.Link.Linked) >= sampleCount {
				add(DiagMissingReference, SevError, loc,
					"sample %q linked sample index %d out of range",
					sh.Name, sh.Link.Linked)
			}
		}
		if sh.LoopStart != sh.LoopEnd {
			loops = append(loops, loopRegion{si, sh.LoopStart, sh.LoopEnd})
		}
	}

	for a := 0; a < len(loops); a++ {
		for b := a + 1; b < len(loops); b++ {
			x, y := loops[a], loops[b]
			if x.start < y.end && y.start < x.end {
				add(DiagOverlappingLoop, SevError,
					fmt.Sprintf("shdr[%d]", x.idx),
					"loop [%d,%d) overlaps shdr[%d] loop [%d,%d)",
					x.start, x.end, y.idx, y.start, y.end)
			}
		}
	}
}
