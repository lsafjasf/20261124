package fixture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"hdr10audit/hdr"
)

type Options struct {
	ContainerMDCV []hdr.Mastering
	ContainerCLLI []hdr.ContentLight
	Bitstream     []hdr.Metadata
	ExtraSample   []byte
}

func DefaultMetadata() (hdr.Mastering, hdr.ContentLight) {
	m := hdr.Mastering{
		Primaries: hdr.Primaries{
			{X: 13250, Y: 34500},
			{X: 7500, Y: 3000},
			{X: 34000, Y: 16000},
		},
		WhitePoint:   hdr.Point{X: 15635, Y: 16450},
		MaxLuminance: 10000000,
		MinLuminance: 1,
	}
	return m, hdr.ContentLight{MaxCLL: 1000, MaxFALL: 400}
}

func Write(path string, opts Options) error {
	data, err := Build(opts)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Build(opts Options) ([]byte, error) {
	sample := buildSample(opts)
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isomiso5hvc1"))
	moovZero := buildMoov(opts, 0, len(sample))
	mdatOffset := len(ftyp) + len(moovZero) + 8
	moov := buildMoov(opts, mdatOffset, len(sample))
	if len(moov) != len(moovZero) {
		return nil, fmt.Errorf("moov size changed after stco patch")
	}
	mdat := box("mdat", sample)
	out := bytes.Join([][]byte{ftyp, moov, mdat}, nil)
	return out, nil
}

func buildMoov(opts Options, sampleOffset, sampleSize int) []byte {
	mvhd := box("mvhd", buildMVHD())
	tkhd := box("tkhd", buildTKHD())
	mdhd := box("mdhd", buildMDHD())
	hdlr := box("hdlr", buildHDLR())
	vmhd := box("vmhd", buildVMHD())
	dinf := box("dinf", buildDREF())
	stbl := box("stbl", bytes.Join([][]byte{
		buildSTSD(opts),
		box("stts", encodeStts()),
		box("stsc", encodeStsc()),
		box("stsz", encodeStsz(sampleSize)),
		box("stco", encodeStco(sampleOffset)),
	}, nil))
	minf := box("minf", bytes.Join([][]byte{vmhd, dinf, stbl}, nil))
	mdia := box("mdia", bytes.Join([][]byte{mdhd, hdlr, minf}, nil))
	trak := box("trak", bytes.Join([][]byte{tkhd, mdia}, nil))
	return box("moov", bytes.Join([][]byte{mvhd, trak}, nil))
}

func buildMVHD() []byte {
	p := make([]byte, 100)
	binary.BigEndian.PutUint32(p[12:16], 1000)
	binary.BigEndian.PutUint32(p[16:20], 1)
	binary.BigEndian.PutUint32(p[20:24], 0x00010000)
	binary.BigEndian.PutUint32(p[24:28], 0x01000000)
	putUnityMatrix(p[36:72])
	binary.BigEndian.PutUint32(p[96:100], 2)
	return p
}

func buildTKHD() []byte {
	p := make([]byte, 84)
	binary.BigEndian.PutUint32(p[12:16], 1)
	binary.BigEndian.PutUint32(p[20:24], 1)
	putUnityMatrix(p[40:76])
	binary.BigEndian.PutUint32(p[76:80], 64<<16)
	binary.BigEndian.PutUint32(p[80:84], 64<<16)
	return p
}

func buildMDHD() []byte {
	p := make([]byte, 24)
	binary.BigEndian.PutUint32(p[12:16], 1000)
	binary.BigEndian.PutUint32(p[16:20], 1)
	binary.BigEndian.PutUint16(p[20:22], 0x55c4)
	return p
}

func buildHDLR() []byte {
	p := make([]byte, 24)
	copy(p[8:12], "vide")
	p = append(p, []byte("VideoHandler\x00")...)
	return p
}

func buildVMHD() []byte {
	p := make([]byte, 12)
	binary.BigEndian.PutUint32(p[0:4], 1)
	return p
}

func buildDREF() []byte {
	url := box("url ", []byte{0, 0, 0, 1})
	p := make([]byte, 8)
	binary.BigEndian.PutUint32(p[4:8], 1)
	return box("dref", append(p, url...))
}

func putUnityMatrix(p []byte) {
	binary.BigEndian.PutUint32(p[0:4], 0x00010000)
	binary.BigEndian.PutUint32(p[16:20], 0x00010000)
	binary.BigEndian.PutUint32(p[32:36], 0x40000000)
}

func buildSTSD(opts Options) []byte {
	entryPayload := make([]byte, 78)
	binary.BigEndian.PutUint16(entryPayload[6:8], 1)
	binary.BigEndian.PutUint16(entryPayload[24:26], 64)
	binary.BigEndian.PutUint16(entryPayload[26:28], 64)
	binary.BigEndian.PutUint32(entryPayload[28:32], 0x00480000)
	binary.BigEndian.PutUint32(entryPayload[32:36], 0x00480000)
	binary.BigEndian.PutUint16(entryPayload[40:42], 1)
	binary.BigEndian.PutUint16(entryPayload[74:76], 0x0018)
	binary.BigEndian.PutUint16(entryPayload[76:78], 0xffff)
	children := [][]byte{box("hvcC", emptyHEVCC())}
	for _, m := range opts.ContainerMDCV {
		children = append(children, box("mdcv", hdr.EncodeMastering(m)))
	}
	for _, c := range opts.ContainerCLLI {
		children = append(children, box("clli", hdr.EncodeContentLight(c)))
	}
	entryPayload = append(entryPayload, bytes.Join(children, nil)...)
	entry := box("hvc1", entryPayload)
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[4:8], 1)
	payload = append(payload, entry...)
	return box("stsd", payload)
}

func emptyHEVCC() []byte {
	data := make([]byte, 23)
	data[0] = 1
	data[21] = 0x03
	data[22] = 0
	return data
}

func buildSample(opts Options) []byte {
	var out []byte
	for _, md := range opts.Bitstream {
		rbsp := buildSEIMessages(md)
		nal := append([]byte{0x4e, 0x01}, escapeRBSP(append(rbsp, 0x80))...)
		out = append(out, lengthPrefixed(nal)...)
	}
	vcl := []byte{0x26, 0x01, 0xde, 0xad, 0xbe, 0xef}
	out = append(out, lengthPrefixed(vcl)...)
	out = append(out, opts.ExtraSample...)
	return out
}

func buildSEIMessages(md hdr.Metadata) []byte {
	var out []byte
	if md.Mastering != nil {
		out = append(out, 137, 24)
		out = append(out, hdr.EncodeMastering(*md.Mastering)...)
	}
	if md.Light != nil {
		out = append(out, 144, 4)
		out = append(out, hdr.EncodeContentLight(*md.Light)...)
	}
	return out
}

func lengthPrefixed(nal []byte) []byte {
	out := make([]byte, 4+len(nal))
	binary.BigEndian.PutUint32(out[:4], uint32(len(nal)))
	copy(out[4:], nal)
	return out
}

func escapeRBSP(data []byte) []byte {
	out := make([]byte, 0, len(data)+8)
	zeros := 0
	for _, b := range data {
		if zeros >= 2 && b <= 0x03 {
			out = append(out, 0x03)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func encodeStts() []byte {
	p := make([]byte, 16)
	binary.BigEndian.PutUint32(p[4:8], 1)
	binary.BigEndian.PutUint32(p[8:12], 1)
	binary.BigEndian.PutUint32(p[12:16], 1)
	return p
}

func encodeStsc() []byte {
	p := make([]byte, 20)
	binary.BigEndian.PutUint32(p[4:8], 1)
	binary.BigEndian.PutUint32(p[8:12], 1)
	binary.BigEndian.PutUint32(p[12:16], 1)
	binary.BigEndian.PutUint32(p[16:20], 1)
	return p
}

func encodeStsz(sampleSize int) []byte {
	p := make([]byte, 12)
	binary.BigEndian.PutUint32(p[4:8], uint32(sampleSize))
	binary.BigEndian.PutUint32(p[8:12], 1)
	return p
}

func encodeStco(offset int) []byte {
	p := make([]byte, 16)
	binary.BigEndian.PutUint32(p[4:8], 1)
	binary.BigEndian.PutUint32(p[8:12], uint32(offset))
	return p
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}
