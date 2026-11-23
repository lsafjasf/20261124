package sf2

// Diagnostic categories. Every problem found while walking the tables is
// tagged with exactly one category so callers can group the report.
const (
	DiagMissingReference = "missing-reference"
	DiagOverlappingLoop  = "overlapping-loop"
	DiagIllegalRootKey   = "illegal-root-key"
	DiagStructure        = "structure"
)

// Severity levels retained on every diagnostic.
const (
	SevError   = "error"
	SevWarning = "warning"
)

// Diagnostic is one located problem. Location strings identify the table and
// record index, e.g. "pgen[7]" or "shdr[3]".
type Diagnostic struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Location string `json:"location"`
	Message  string `json:"message"`
}

// Range is an inclusive MIDI key or velocity range.
type Range struct {
	Low  int `json:"low"`
	High int `json:"high"`
}

// GenValue is a generator as applied on a resolved sample zone. Unknown
// generators are preserved verbatim: ID is the raw sfGenOper and Amount is
// the raw signed 16 bit value.
type GenValue struct {
	ID     uint16 `json:"id"`
	Name   string `json:"name"`
	Amount int16  `json:"amount"`
	Known  bool   `json:"known"`
}

// LoopPoints carries both the values declared in shdr and the effective loop
// after fine/coarse address generators have been applied.
type LoopPoints struct {
	DeclaredStart  uint32 `json:"declared_start"`
	DeclaredEnd    uint32 `json:"declared_end"`
	EffectiveStart uint32 `json:"effective_start"`
	EffectiveEnd   uint32 `json:"effective_end"`
	Mode           int    `json:"mode"` // 0 no loop, 1 continuous, 3 until release
}

// ResolvedSample is one sample reached from one instrument zone of one
// preset. The same shdr record can appear many times under different presets.
type ResolvedSample struct {
	PresetIndex     int        `json:"preset_index"`
	PresetName      string     `json:"preset_name"`
	Bank            uint16     `json:"bank"`
	Preset          uint16     `json:"preset"`
	InstrumentIndex int        `json:"instrument_index"`
	InstrumentName  string     `json:"instrument_name"`
	SampleIndex     int        `json:"sample_index"`
	SampleName      string     `json:"sample_name"`
	KeyRange        Range      `json:"key_range"`
	VelRange        Range      `json:"vel_range"`
	RootKey         int        `json:"root_key"` // -1 when unspecified
	Loop            LoopPoints `json:"loop"`
	Generators      []GenValue `json:"generators"`
}

// Report is the full inspection result.
type Report struct {
	Resolved    []ResolvedSample `json:"resolved"`
	Diagnostics []Diagnostic     `json:"diagnostics"`
}

type zone struct {
	gens     []GenRecord
	keyRange Range
	velRange Range
	hasKey   bool
	hasVel   bool
}

func defaultKeyRange() Range { return Range{Low: 0, High: 127} }
func defaultVelRange() Range { return Range{Low: 0, High: 127} }

// splitZones divides a generator slice belonging to bags[start:end] into
// per-bag zones. The first zone is "global" when it carries no index
// generator (instrument/sample id); callers merge it into following zones.
func splitZones(gens []GenRecord, bags []bagRecord, start, end int) []zone {
	// end is the index of the terminal bag. Playable zones are bags
	// [start,end); the terminal bag supplies the final generator upper bound.
	if end <= start || end > len(bags) {
		return nil
	}
	zones := make([]zone, 0, end-start)
	for bag := start; bag < end; bag++ {
		genStart := int(bags[bag].GenNdx)
		genEnd := int(bags[bag+1].GenNdx)
		if genStart > genEnd || genEnd > len(gens) {
			genStart, genEnd = 0, 0
		}
		z := zone{
			gens:     gens[genStart:genEnd:genEnd],
			keyRange: defaultKeyRange(),
			velRange: defaultVelRange(),
		}
		for _, g := range z.gens {
			switch g.ID {
			case GenKeyRange:
				z.hasKey = true
				z.keyRange = Range{Low: int(uint16(g.AmountRaw) & 0xff), High: int(uint16(g.AmountRaw) >> 8)}
			case GenVelRange:
				z.hasVel = true
				z.velRange = Range{Low: int(uint16(g.AmountRaw) & 0xff), High: int(uint16(g.AmountRaw) >> 8)}
			}
		}
		zones = append(zones, z)
	}
	return zones
}

func zoneHasInstrument(z zone) bool {
	for _, g := range z.gens {
		if isInstrumentIndexGen(g.ID) {
			return true
		}
	}
	return false
}

func zoneHasSample(z zone) bool {
	for _, g := range z.gens {
		if isSampleIndexGen(g.ID) {
			return true
		}
	}
	return false
}
