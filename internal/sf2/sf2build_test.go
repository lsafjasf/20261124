package sf2

import (
	"bytes"
	"encoding/binary"
)

// sf2Builder assembles a minimal but structurally valid SoundFont 2 file in
// memory. Tests append records in table order; terminal records are added
// explicitly so individual tests can corrupt or omit them on purpose.
type sf2Builder struct {
	presets []PresetHeader
	pbags   []PresetBag
	pgens   []GenRecord
	insts   []InstHeader
	ibags   []InstBag
	igens   []GenRecord
	samples []SampleHeader
	smpl    []byte
}

func put16(b []byte, v uint16) { binary.LittleEndian.PutUint16(b, v) }
func put32(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }

func putName(b []byte, name string) {
	copy(b, name)
}

func gen(id uint16, amount int16) GenRecord { return GenRecord{ID: id, AmountRaw: amount} }
func genIndex(id uint16, idx uint16) GenRecord {
	return GenRecord{ID: id, AmountRaw: int16(idx)}
}

func (b *sf2Builder) bytes() []byte {
	var pdta bytes.Buffer
	writeChunk(&pdta, "phdr", b.phdrData())
	writeChunk(&pdta, "pbag", b.bagData(true))
	writeChunk(&pdta, "pgen", b.genData(true))
	writeChunk(&pdta, "inst", b.instData())
	writeChunk(&pdta, "ibag", b.bagData(false))
	writeChunk(&pdta, "igen", b.genData(false))
	writeChunk(&pdta, "shdr", b.shdrData())

	var sdta bytes.Buffer
	writeChunk(&sdta, "smpl", b.smpl)

	var content bytes.Buffer
	writeList(&content, "INFO", nil)
	writeList(&content, "sdta", sdta.Bytes())
	writeList(&content, "pdta", pdta.Bytes())

	out := append([]byte("RIFF"), make([]byte, 4)...)
	put32(out[4:], uint32(content.Len()+4))
	out = append(out, "sfbk"...)
	out = append(out, content.Bytes()...)
	return out
}

func writeChunk(w *bytes.Buffer, id string, payload []byte) {
	w.WriteString(id)
	hdr := make([]byte, 4)
	put32(hdr, uint32(len(payload)))
	w.Write(hdr)
	w.Write(payload)
	if len(payload)%2 == 1 {
		w.WriteByte(0)
	}
}

func writeList(w *bytes.Buffer, typ string, body []byte) {
	w.WriteString("LIST")
	hdr := make([]byte, 4)
	put32(hdr, uint32(len(body)+4))
	w.Write(hdr)
	w.WriteString(typ)
	w.Write(body)
}

func (b *sf2Builder) phdrData() []byte {
	var buf bytes.Buffer
	for _, p := range b.presets {
		rec := make([]byte, 38)
		putName(rec[0:20], p.Name)
		put16(rec[20:22], p.Preset)
		put16(rec[22:24], p.Bank)
		put16(rec[24:26], p.PresetBagIdx)
		put32(rec[26:30], p.Library)
		put32(rec[30:34], p.Genre)
		put32(rec[34:38], p.Morphology)
		buf.Write(rec)
	}
	return buf.Bytes()
}

func (b *sf2Builder) instData() []byte {
	var buf bytes.Buffer
	for _, in := range b.insts {
		rec := make([]byte, 22)
		putName(rec[0:20], in.Name)
		put16(rec[20:22], in.InstBagIdx)
		buf.Write(rec)
	}
	return buf.Bytes()
}

func (b *sf2Builder) bagData(preset bool) []byte {
	var buf bytes.Buffer
	if preset {
		for _, bg := range b.pbags {
			rec := make([]byte, 4)
			put16(rec[0:2], bg.GenNdx)
			put16(rec[2:4], bg.ModNdx)
			buf.Write(rec)
		}
		return buf.Bytes()
	}
	for _, bg := range b.ibags {
		rec := make([]byte, 4)
		put16(rec[0:2], bg.GenNdx)
		put16(rec[2:4], bg.ModNdx)
		buf.Write(rec)
	}
	return buf.Bytes()
}

func (b *sf2Builder) genData(preset bool) []byte {
	gens := b.igens
	if preset {
		gens = b.pgens
	}
	var buf bytes.Buffer
	for _, g := range gens {
		rec := make([]byte, 4)
		put16(rec[0:2], g.ID)
		put16(rec[2:4], uint16(g.AmountRaw))
		buf.Write(rec)
	}
	return buf.Bytes()
}

func (b *sf2Builder) shdrData() []byte {
	var buf bytes.Buffer
	for _, s := range b.samples {
		rec := make([]byte, 46)
		putName(rec[0:20], s.Name)
		put32(rec[20:24], s.Start)
		put32(rec[24:28], s.End)
		put32(rec[28:32], s.LoopStart)
		put32(rec[32:36], s.LoopEnd)
		put32(rec[36:40], s.Rate)
		rec[40] = s.RootKey
		rec[41] = byte(s.Correction)
		put16(rec[42:44], s.Link.Linked)
		put16(rec[44:46], s.Link.Type)
		buf.Write(rec)
	}
	return buf.Bytes()
}
