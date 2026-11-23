package sf2

// Raw SoundFont 2 chunk records. Every integer field is little-endian on disk.
// Sizes match the SF2 specification exactly: phdr=38, pbag/ibag=4,
// pgen/igen=4, inst=22, shdr=46 bytes per record.

// PresetHeader is one phdr record.
type PresetHeader struct {
	Name         string // achPresetName, NUL padded
	Preset       uint16 // wPreset
	Bank         uint16 // wBank
	PresetBagIdx uint16 // dwPresetBagNdx (only low 16 bits are used)
	Library      uint32 // dwLibrary
	Genre        uint32 // dwGenre
	Morphology   uint32 // dwMorphology
}

// PresetBag is one pbag record.
type PresetBag struct {
	GenNdx uint16 // dwGenNdx
	ModNdx uint16 // dwModNdx
}

// InstHeader is one inst record.
type InstHeader struct {
	Name       string // achInstName, NUL padded
	InstBagIdx uint16 // dwInstBagNdx
}

// InstBag is one ibag record.
type InstBag struct {
	GenNdx uint16 // dwGenNdx
	ModNdx uint16 // dwModNdx
}

// GenRecord is one pgen or igen record: a generator id plus a 16 bit raw
// amount. AmountRaw keeps the original signed value; for index generators the
// value is reinterpreted as an unsigned index by callers.
type GenRecord struct {
	ID        uint16 // sfGenOper
	AmountRaw int16  // genAmount (signed 16 bit)
}

// AmountIndex returns the raw amount interpreted as an unsigned index, which
// is how range/index generators (sampleID, instrumentID, ...) are encoded.
func (g GenRecord) AmountIndex() uint16 { return uint16(g.AmountRaw) }

// SampleLink describes the two linked sample indices of an SHDR record.
type SampleLink struct {
	Linked uint16 // wSampleLink
	Type   uint16 // sfSampleType
}

// SampleHeader is one shdr record. All offsets are in sample frames relative
// to the start of the smpl (or sm24 for 24 bit) sample data chunk.
type SampleHeader struct {
	Name       string // achSampleName, NUL padded
	Start      uint32 // dwStart
	End        uint32 // dwEnd
	LoopStart  uint32 // dwStartloop
	LoopEnd    uint32 // dwEndloop
	Rate       uint32 // dwSampleRate
	RootKey    byte   // byOriginalPitch, 255 means unspecified
	Correction int8   // chPitchCorrection in cents
	Link       SampleLink
}

// Sample type flags (sfSampleType).
const (
	SampleTypeMono   uint16 = 1
	SampleTypeRight  uint16 = 2
	SampleTypeLeft   uint16 = 4
	SampleTypeLinked uint16 = 8
	SampleTypeROM    uint16 = 0x8000
)

// File holds the decoded chunk tables plus the size of the sample data chunk,
// which is enough to validate shdr frame offsets.
type File struct {
	Presets []PresetHeader
	PBags   []PresetBag
	PGens   []GenRecord
	Insts   []InstHeader
	IBags   []InstBag
	IGens   []GenRecord
	Samples []SampleHeader

	// SmplFrames is the number of 16 bit sample frames covered by the smpl
	// chunk (0 when the chunk is absent).
	SmplFrames uint32
}
