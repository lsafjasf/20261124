package sf2

// Generator identifiers (sfGenOper), numbered per the SoundFont 2.04 spec
// section 8.1.3. Vintage sound fonts encode instrument/sample indices at
// 41/37 while the 2.04 table also lists 49/53; both are accepted.
const (
	GenStartAddrOffset           uint16 = 0
	GenEndAddrOffset             uint16 = 1
	GenStartLoopAddrOffset       uint16 = 2
	GenEndLoopAddrOffset         uint16 = 3
	GenStartAddrCoarseOffset     uint16 = 4
	GenEndAddrCoarseOffset       uint16 = 5
	GenKeyRange                  uint16 = 43
	GenVelRange                  uint16 = 44
	GenStartLoopAddrCoarseOffset uint16 = 45
	GenEndLoopAddrCoarseOffset   uint16 = 46
	GenKeyNum                    uint16 = 47
	GenVelocity                  uint16 = 48
	GenInstrumentID              uint16 = 41
	GenInstrumentID204           uint16 = 49
	GenSampleID                  uint16 = 53
	GenSampleIDClassic           uint16 = 37
	GenSampleModes               uint16 = 54
	GenOverridingRootKey         uint16 = 58
)

// isInstrumentIndexGen accepts the classic id 41 and, for 2.04 files, id 49.
func isInstrumentIndexGen(id uint16) bool {
	return id == GenInstrumentID || id == GenInstrumentID204
}

// isSampleIndexGen accepts the classic id 37 and, for 2.04 files, id 53.
func isSampleIndexGen(id uint16) bool {
	return id == GenSampleID || id == GenSampleIDClassic
}

// genKind classifies how a generator amount is interpreted.
type genKind int

const (
	kindUnknown genKind = iota
	kindIndex
	kindSigned
	kindRange
	kindRootKey
)

type genInfo struct {
	Name string
	Kind genKind
}

// generatorTable is indexed by the raw sfGenOper id (0..60). IDs outside the
// table are unknown generators: the parser keeps their raw id/amount even
// though it has no semantics for them.
var generatorTable = []genInfo{
	{Name: "startAddrsOffset", Kind: kindSigned},           // 0
	{Name: "endAddrsOffset", Kind: kindSigned},             // 1
	{Name: "startloopAddrsOffset", Kind: kindSigned},       // 2
	{Name: "endloopAddrsOffset", Kind: kindSigned},         // 3
	{Name: "startAddrsCoarseOffset", Kind: kindSigned},     // 4
	{Name: "endAddrsCoarseOffset", Kind: kindSigned},       // 5
	{Name: "reserved6", Kind: kindUnknown},                 // 6
	{Name: "reserved7", Kind: kindUnknown},                 // 7
	{Name: "pan", Kind: kindSigned},                        // 8
	{Name: "delayVolEnv", Kind: kindSigned},                // 9
	{Name: "attackVolEnv", Kind: kindSigned},               // 10
	{Name: "holdVolEnv", Kind: kindSigned},                 // 11
	{Name: "decayVolEnv", Kind: kindSigned},                // 12
	{Name: "sustainVolEnv", Kind: kindSigned},              // 13
	{Name: "releaseVolEnv", Kind: kindSigned},              // 14
	{Name: "holdModEnv", Kind: kindSigned},                 // 15
	{Name: "decayModEnv", Kind: kindSigned},                // 16
	{Name: "sustainModEnv", Kind: kindSigned},              // 17
	{Name: "releaseModEnv", Kind: kindSigned},              // 18
	{Name: "delayModEnv", Kind: kindSigned},                // 19
	{Name: "attackModEnv", Kind: kindSigned},               // 20
	{Name: "keyNumToModEnvHold", Kind: kindSigned},         // 21
	{Name: "keyNumToModEnvDecay", Kind: kindSigned},        // 22
	{Name: "keyNumToVolEnvHold", Kind: kindSigned},         // 23
	{Name: "keyNumToVolEnvDecay", Kind: kindSigned},        // 24
	{Name: "reserved25", Kind: kindUnknown},                // 25
	{Name: "reserved26", Kind: kindUnknown},                // 26
	{Name: "keyRange", Kind: kindRange},                    // 27
	{Name: "velRange", Kind: kindRange},                    // 28
	{Name: "reserved29", Kind: kindUnknown},                // 29
	{Name: "keynum", Kind: kindRootKey},                    // 30
	{Name: "velocity", Kind: kindSigned},                   // 31
	{Name: "initialAttenuation", Kind: kindSigned},         // 32
	{Name: "reserved33", Kind: kindUnknown},                // 33
	{Name: "reserved34", Kind: kindUnknown},                // 34
	{Name: "coarseTune", Kind: kindSigned},                 // 35
	{Name: "fineTune", Kind: kindSigned},                   // 36
	{Name: "sampleID", Kind: kindIndex},                    // 37
	{Name: "sampleModes", Kind: kindSigned},                // 38
	{Name: "reserved39", Kind: kindUnknown},                // 39
	{Name: "scaleTuning", Kind: kindSigned},                // 40
	{Name: "instrument", Kind: kindIndex},                  // 41
	{Name: "reserved42", Kind: kindUnknown},                // 42
	{Name: "keyRange", Kind: kindRange},                    // 43
	{Name: "velRange", Kind: kindRange},                    // 44
	{Name: "startloopAddrsCoarseOffset", Kind: kindSigned}, // 45
	{Name: "endloopAddrsCoarseOffset", Kind: kindSigned},   // 46
	{Name: "keynum", Kind: kindRootKey},                    // 47
	{Name: "velocity", Kind: kindSigned},                   // 48
	{Name: "instrument49", Kind: kindIndex},                // 49
	{Name: "reserved50", Kind: kindUnknown},                // 50
	{Name: "reserved51", Kind: kindUnknown},                // 51
	{Name: "cutoff", Kind: kindSigned},                     // 52
	{Name: "sampleID204", Kind: kindIndex},                 // 53
	{Name: "sampleModes204", Kind: kindSigned},             // 54
	{Name: "reserved55", Kind: kindUnknown},                // 55
	{Name: "reserved56", Kind: kindUnknown},                // 56
	{Name: "reserved57", Kind: kindUnknown},                // 57
	{Name: "overridingRootKey", Kind: kindRootKey},         // 58
	{Name: "reserved59", Kind: kindUnknown},                // 59
	{Name: "endOper", Kind: kindUnknown},                   // 60
}

// GeneratorName returns the spec name for a generator id, or a synthesized
// "unknownNN" label when the id is not part of the SF2.04 table.
func GeneratorName(id uint16) string {
	if int(id) < len(generatorTable) {
		return generatorTable[id].Name
	}
	return "unknown" + itoa(id)
}

// IsKnownGenerator reports whether id is inside the SF2.04 enumeration.
// Reserved ids count as known but semantically unused; only ids past 60 are
// truly unknown and preserved verbatim.
func IsKnownGenerator(id uint16) bool {
	return int(id) < len(generatorTable)
}

func itoa(v uint16) string {
	if v == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
