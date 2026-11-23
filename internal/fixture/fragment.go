package fixture

import (
	"bytes"
	"encoding/binary"
)

func BuildFragmented(opts Options) ([]byte, error) {
	sample := buildSample(opts)
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isomiso5hvc1"))
	moov := buildFragmentInit(opts)
	moofPlaceholder := buildMoof(0, len(sample))
	dataOffset := len(moofPlaceholder) + 8
	moof := buildMoof(dataOffset, len(sample))
	if len(moof) != len(moofPlaceholder) {
		return nil, errSizeChanged
	}
	return bytes.Join([][]byte{ftyp, moov, moof, box("mdat", sample)}, nil), nil
}

func buildMoof(dataOffset, sampleSize int) []byte {
	mfhd := box("mfhd", make([]byte, 8))
	tfhdPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(tfhdPayload[0:4], 0x00020000)
	binary.BigEndian.PutUint32(tfhdPayload[4:8], 1)
	tfhd := box("tfhd", tfhdPayload)
	trunPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(trunPayload[0:4], 0x00000201)
	binary.BigEndian.PutUint32(trunPayload[4:8], 1)
	binary.BigEndian.PutUint32(trunPayload[8:12], uint32(dataOffset))
	binary.BigEndian.PutUint32(trunPayload[12:16], uint32(sampleSize))
	trun := box("trun", trunPayload)
	traf := box("traf", bytes.Join([][]byte{tfhd, trun}, nil))
	return box("moof", bytes.Join([][]byte{mfhd, traf}, nil))
}

func buildFragmentInit(opts Options) []byte {
	mvhd := box("mvhd", buildMVHD())
	tkhd := box("tkhd", buildTKHD())
	mdhd := box("mdhd", buildMDHD())
	hdlr := box("hdlr", buildHDLR())
	vmhd := box("vmhd", buildVMHD())
	dinf := box("dinf", buildDREF())
	stsd := buildSTSD(opts)
	stbl := box("stbl", bytes.Join([][]byte{
		stsd,
		box("stts", make([]byte, 8)),
		box("stsc", make([]byte, 8)),
		box("stsz", make([]byte, 12)),
		box("stco", make([]byte, 8)),
	}, nil))
	minf := box("minf", bytes.Join([][]byte{vmhd, dinf, stbl}, nil))
	mdia := box("mdia", bytes.Join([][]byte{mdhd, hdlr, minf}, nil))
	trak := box("trak", bytes.Join([][]byte{tkhd, mdia}, nil))
	return box("moov", bytes.Join([][]byte{mvhd, trak}, nil))
}

var errSizeChanged = sizeError("fixture box size changed")

type sizeError string

func (e sizeError) Error() string { return string(e) }
