package sf2

// bagSpan returns the half-open bag index range [start,end) owned by one
// phdr/inst record and emits a structural diagnostic when the indices are
// inconsistent with the bag table length.
func bagSpan(first, next uint16, bagLen int, loc string,
	add func(string, string, string, string, ...any)) (int, int) {
	start, end := int(first), int(next)
	if start > bagLen {
		add(DiagStructure, SevError, loc,
			"bag start index %d exceeds bag table length %d", start, bagLen)
		return 0, 0
	}
	if end > bagLen || end < start {
		add(DiagStructure, SevError, loc,
			"bag span [%d,%d) invalid for table length %d", start, end, bagLen)
		return start, minInt(bagLen, maxInt(start, end))
	}
	return start, end
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractGlobalPreset splits preset zones into an optional global zone (the
// first zone when it has no instrument generator) and the local zones.
func extractGlobalPreset(zones []zone) (zone, []zone) {
	if len(zones) > 0 && !zoneHasInstrument(zones[0]) {
		return zones[0], zones[1:]
	}
	empty := zone{
		keyRange: defaultKeyRange(),
		velRange: defaultVelRange(),
	}
	return empty, zones
}

// extractGlobalInstrument splits instrument zones on the sampleID generator.
func extractGlobalInstrument(zones []zone) (zone, []zone) {
	if len(zones) > 0 && !zoneHasSample(zones[0]) {
		return zones[0], zones[1:]
	}
	empty := zone{
		keyRange: defaultKeyRange(),
		velRange: defaultVelRange(),
	}
	return empty, zones
}

func findGen(gens []GenRecord, id uint16) *GenRecord {
	for i := range gens {
		if gens[i].ID == id {
			return &gens[i]
		}
	}
	return nil
}

func findIndexGen(gens []GenRecord, match func(uint16) bool) *GenRecord {
	for i := range gens {
		if match(gens[i].ID) {
			return &gens[i]
		}
	}
	return nil
}

// mergeGens layers preset-level generators under instrument-level generators.
// Per SF2 semantics an instrument-level generator wins when both layers set
// the same id; unknown generators are merged and preserved identically.
func mergeGens(lower, upper []GenRecord) []GenRecord {
	indexOf := func(list []GenRecord, id uint16) int {
		for i := range list {
			if list[i].ID == id {
				return i
			}
		}
		return -1
	}
	out := append([]GenRecord(nil), lower...)
	for _, g := range upper {
		if pos := indexOf(out, g.ID); pos >= 0 {
			out[pos] = g
			continue
		}
		out = append(out, g)
	}
	return out
}

func toGenValues(gens []GenRecord) []GenValue {
	out := make([]GenValue, 0, len(gens))
	for _, g := range gens {
		out = append(out, GenValue{
			ID:     g.ID,
			Name:   GeneratorName(g.ID),
			Amount: g.AmountRaw,
			Known:  IsKnownGenerator(g.ID),
		})
	}
	return out
}

func addrOffset(gens []GenRecord, fineID, coarseID uint16) int64 {
	var off int64
	if g := findGen(gens, fineID); g != nil {
		off += int64(g.AmountRaw)
	}
	if g := findGen(gens, coarseID); g != nil {
		off += int64(g.AmountRaw) * 32768
	}
	return off
}

func sampleMode(gens []GenRecord) int {
	// sampleModes is id 54 in both the deployed files and SF2.04; id 38 is
	// only a historical alias, so prefer the canonical location.
	if g := findGen(gens, GenSampleModes); g != nil {
		return int(g.AmountRaw)
	}
	return 0
}

// computeLoop applies the fine/coarse loop address generators to the shdr
// loop points and clamps nothing: out-of-range results are reported later.
func computeLoop(sh SampleHeader, gens []GenRecord) LoopPoints {
	ls := int64(sh.LoopStart) + addrOffset(gens, GenStartLoopAddrOffset, GenStartLoopAddrCoarseOffset)
	le := int64(sh.LoopEnd) + addrOffset(gens, GenEndLoopAddrOffset, GenEndLoopAddrCoarseOffset)
	return LoopPoints{
		DeclaredStart:  sh.LoopStart,
		DeclaredEnd:    sh.LoopEnd,
		EffectiveStart: uint32(maxInt64(ls, 0)),
		EffectiveEnd:   uint32(maxInt64(le, 0)),
		Mode:           sampleMode(gens),
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
