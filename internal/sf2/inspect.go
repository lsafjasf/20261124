package sf2

import "fmt"

// Inspect validates cross-table references, expands every preset down to the
// concrete samples it plays and locates loop/root-key problems.
func (f *File) Inspect() *Report {
	rep := &Report{}
	add := func(cat, sev, loc, format string, args ...any) {
		rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
			Category: cat, Severity: sev, Location: loc,
			Message: fmt.Sprintf(format, args...),
		})
	}

	pBags := toBagRecords(f.PBags)
	iBags := toBagRecords(f.IBags)
	f.validateTables(pBags, iBags, add)

	presetCount := len(f.Presets) - 1
	instCount := len(f.Insts) - 1
	sampleCount := len(f.Samples) - 1

	for pi := 0; pi < maxInt(presetCount, 0); pi++ {
		ph := f.Presets[pi]
		var nextBag uint16
		if pi+1 < len(f.Presets) {
			nextBag = f.Presets[pi+1].PresetBagIdx
		} else {
			nextBag = uint16(len(pBags) - 1)
		}
		start, end := bagSpan(ph.PresetBagIdx, nextBag, len(pBags),
			fmt.Sprintf("phdr[%d]", pi), add)
		zones := splitZones(f.PGens, pBags, start, end)
		global, localZones := extractGlobalPreset(zones)
		if len(zones) > 0 && len(localZones) == 0 && !zoneHasInstrument(zones[0]) {
			add(DiagMissingReference, SevError, fmt.Sprintf("phdr[%d]", pi),
				"preset %q has no instrument zone", ph.Name)
		}
		for zi, z := range localZones {
			if !zoneHasInstrument(z) {
				add(DiagStructure, SevWarning, fmt.Sprintf("phdr[%d].pgen zone %d", pi, zi+1),
					"non-global preset zone without instrumentID skipped")
				continue
			}
			instGen := findIndexGen(z.gens, isInstrumentIndexGen)
			if instGen == nil {
				add(DiagMissingReference, SevError, fmt.Sprintf("phdr[%d].pgen zone %d", pi, zi+1),
					"preset zone has no instrumentID generator")
				continue
			}
			instIdx := int(instGen.AmountIndex())
			loc := fmt.Sprintf("phdr[%d].pgen zone %d", pi, zi+1)
			if instIdx < 0 || instIdx >= maxInt(instCount, 0) {
				add(DiagMissingReference, SevError, loc,
					"instrumentID %d out of range (have %d instruments)", instIdx, maxInt(instCount, 0))
				continue
			}
			mergedPreset := mergeGens(global.gens, z.gens)
			f.expandInstrument(rep, add, pi, instIdx, mergedPreset, iBags, sampleCount)
		}
	}

	f.checkSamples(rep, add, maxInt(sampleCount, 0))
	return rep
}

func (f *File) expandInstrument(rep *Report, add func(string, string, string, string, ...any),
	presetIdx, instIdx int, presetGens []GenRecord, iBags []bagRecord, sampleCount int) {
	ih := f.Insts[instIdx]
	var nextBag uint16
	if instIdx+1 < len(f.Insts) {
		nextBag = f.Insts[instIdx+1].InstBagIdx
	} else {
		nextBag = uint16(len(iBags) - 1)
	}
	start, end := bagSpan(ih.InstBagIdx, nextBag, len(iBags),
		fmt.Sprintf("inst[%d]", instIdx), add)
	zones := splitZones(f.IGens, iBags, start, end)
	global, localZones := extractGlobalInstrument(zones)
	presetLoc := fmt.Sprintf("phdr[%d]", presetIdx)
	if len(zones) > 0 && len(localZones) == 0 {
		add(DiagMissingReference, SevError, presetLoc,
			"instrument %q has no sample zone", ih.Name)
	}

	for zi, z := range localZones {
		sampleGen := findIndexGen(z.gens, isSampleIndexGen)
		if sampleGen == nil {
			add(DiagMissingReference, SevError, fmt.Sprintf("inst[%d].igen zone %d", instIdx, zi+1),
				"instrument zone has no sampleID generator")
			continue
		}
		sampleIdx := int(sampleGen.AmountIndex())
		loc := fmt.Sprintf("inst[%d].igen zone %d", instIdx, zi+1)
		if sampleIdx < 0 || sampleIdx >= sampleCount {
			if sampleIdx >= 900 {
				add(DiagMissingReference, SevWarning, loc,
					"sampleID %d is a ROM/unused-zone sentinel, no local sample (have %d)",
					sampleIdx, sampleCount)
			} else {
				add(DiagMissingReference, SevError, loc,
					"sampleID %d out of range (have %d samples)", sampleIdx, sampleCount)
			}
			continue
		}
		instGens := mergeGens(global.gens, z.gens)
		effective := mergeGens(presetGens, instGens)
		sh := f.Samples[sampleIdx]

		kr, vr := resolveRanges(global, z)
		rootKey, rootSource := resolveRootKey(sh, effective)
		if !validRootKey(rootKey) {
			add(DiagIllegalRootKey, SevError, rootSource,
				"sample %q reached from preset %q: root key %d outside 0..127",
				sh.Name, f.Presets[presetIdx].Name, rootKey)
		}

		loop := computeLoop(sh, effective)
		if loop.EffectiveEnd < loop.EffectiveStart {
			add(DiagOverlappingLoop, SevError, fmt.Sprintf("shdr[%d]", sampleIdx),
				"sample %q effective loop end %d precedes start %d after generators",
				sh.Name, loop.EffectiveEnd, loop.EffectiveStart)
		}
		if loop.EffectiveStart < sh.Start || loop.EffectiveEnd > sh.End {
			add(DiagOverlappingLoop, SevError, fmt.Sprintf("shdr[%d]", sampleIdx),
				"sample %q effective loop [%d,%d) falls outside data range [%d,%d)",
				sh.Name, loop.EffectiveStart, loop.EffectiveEnd, sh.Start, sh.End)
		}
		rep.Resolved = append(rep.Resolved, ResolvedSample{
			PresetIndex:     presetIdx,
			PresetName:      f.Presets[presetIdx].Name,
			Bank:            f.Presets[presetIdx].Bank,
			Preset:          f.Presets[presetIdx].Preset,
			InstrumentIndex: instIdx,
			InstrumentName:  ih.Name,
			SampleIndex:     sampleIdx,
			SampleName:      sh.Name,
			KeyRange:        kr,
			VelRange:        vr,
			RootKey:         rootKey,
			Loop:            loop,
			Generators:      toGenValues(effective),
		})
	}
}

func resolveRanges(global, local zone) (Range, Range) {
	kr, vr := local.keyRange, local.velRange
	if !local.hasKey {
		kr = global.keyRange
	}
	if !local.hasVel {
		vr = global.velRange
	}
	return kr, vr
}

func resolveRootKey(sh SampleHeader, gens []GenRecord) (int, string) {
	if g := findGen(gens, GenOverridingRootKey); g != nil {
		return int(uint16(g.AmountRaw)), "gen(overridingRootKey)"
	}
	if g := findGen(gens, GenKeyNum); g != nil {
		return int(uint16(g.AmountRaw)), "gen(keynum)"
	}
	return int(sh.RootKey), "shdr.originalPitch"
}

func validRootKey(key int) bool {
	return (key >= 0 && key <= 127) || key == 255
}

func toBagRecords[T PresetBag | InstBag](in []T) []bagRecord {
	out := make([]bagRecord, len(in))
	for i := range in {
		switch v := any(in[i]).(type) {
		case PresetBag:
			out[i] = bagRecord(v)
		case InstBag:
			out[i] = bagRecord(v)
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
