package sf2

import (
	"encoding/binary"
	"fmt"
)

// bagRecord is the common 4 byte layout shared by pbag and ibag.
type bagRecord struct {
	GenNdx uint16
	ModNdx uint16
}

func parseBags[T PresetBag | InstBag](data []byte) ([]T, error) {
	var zero T
	const rec = 4
	if len(data)%rec != 0 {
		return nil, fmt.Errorf("bag chunk size %d is not a multiple of %d", len(data), rec)
	}
	records := make([]bagRecord, len(data)/rec)
	for i := range records {
		r := data[i*rec:]
		records[i] = bagRecord{
			GenNdx: binary.LittleEndian.Uint16(r[0:2]),
			ModNdx: binary.LittleEndian.Uint16(r[2:4]),
		}
	}
	switch any(zero).(type) {
	case PresetBag:
		out := make([]PresetBag, len(records))
		for i, rec := range records {
			out[i] = PresetBag(rec)
		}
		return any(out).([]T), nil
	default:
		out := make([]InstBag, len(records))
		for i, rec := range records {
			out[i] = InstBag(rec)
		}
		return any(out).([]T), nil
	}
}

func parseGens(data []byte) ([]GenRecord, error) {
	const rec = 4
	if len(data)%rec != 0 {
		return nil, fmt.Errorf("gen chunk size %d is not a multiple of %d", len(data), rec)
	}
	out := make([]GenRecord, len(data)/rec)
	for i := range out {
		r := data[i*rec:]
		out[i] = GenRecord{
			ID:        binary.LittleEndian.Uint16(r[0:2]),
			AmountRaw: int16(binary.LittleEndian.Uint16(r[2:4])),
		}
	}
	return out, nil
}

func parseSHDR(data []byte) ([]SampleHeader, error) {
	const rec = 46
	if len(data)%rec != 0 {
		return nil, fmt.Errorf("shdr size %d is not a multiple of %d", len(data), rec)
	}
	out := make([]SampleHeader, 0, len(data)/rec)
	for off := 0; off < len(data); off += rec {
		r := data[off:]
		out = append(out, SampleHeader{
			Name:       fixedString(r[0:20]),
			Start:      binary.LittleEndian.Uint32(r[20:24]),
			End:        binary.LittleEndian.Uint32(r[24:28]),
			LoopStart:  binary.LittleEndian.Uint32(r[28:32]),
			LoopEnd:    binary.LittleEndian.Uint32(r[32:36]),
			Rate:       binary.LittleEndian.Uint32(r[36:40]),
			RootKey:    r[40],
			Correction: int8(r[41]),
			Link: SampleLink{
				Linked: binary.LittleEndian.Uint16(r[42:44]),
				Type:   binary.LittleEndian.Uint16(r[44:46]),
			},
		})
	}
	return out, nil
}
