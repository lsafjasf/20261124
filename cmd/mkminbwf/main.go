// Command mkminbwf writes a minimal, valid ADM BWF fixture: two PCM tracks
// (one DirectSpeakers bed channel, one Objects channel with two position
// blocks), a chna table and an axml document. Used to (re)generate
// testdata/minimal.bwf.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	sampleRate = 48000
	numCh      = 2
	bits       = 16
	seconds    = 1
)

func main() {
	out := "testdata/minimal.bwf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	data := build()
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(data))
}

func chunk(id string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString(id)
	binary.Write(&b, binary.LittleEndian, uint32(len(payload)))
	b.Write(payload)
	if len(payload)%2 != 0 {
		b.WriteByte(0)
	}
	return b.Bytes()
}

func build() []byte {
	var fmtPayload bytes.Buffer
	binary.Write(&fmtPayload, binary.LittleEndian, uint16(1))          // PCM
	binary.Write(&fmtPayload, binary.LittleEndian, uint16(numCh))      // channels
	binary.Write(&fmtPayload, binary.LittleEndian, uint32(sampleRate)) // sample rate
	binary.Write(&fmtPayload, binary.LittleEndian, uint32(sampleRate*numCh*bits/8))
	binary.Write(&fmtPayload, binary.LittleEndian, uint16(numCh*bits/8)) // block align
	binary.Write(&fmtPayload, binary.LittleEndian, uint16(bits))         // bits/sample

	silence := make([]byte, sampleRate*seconds*numCh*bits/8)

	body := bytes.Join([][]byte{
		chunk("fmt ", fmtPayload.Bytes()),
		chunk("axml", []byte(axml)),
		chunk("chna", chna()),
		chunk("data", silence),
	}, nil)

	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, uint32(4+len(body)))
	out.WriteString("WAVE")
	out.Write(body)
	return out.Bytes()
}

func chnaRow(track uint16, uid, trackRef, packRef string) []byte {
	row := make([]byte, 2+12+14+11)
	binary.LittleEndian.PutUint16(row[0:2], track)
	copy(row[2:14], uid)
	copy(row[14:28], trackRef)
	copy(row[28:39], packRef)
	return row
}

func chna() []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint16(2)) // numTracks
	binary.Write(&b, binary.LittleEndian, uint16(2)) // numUIDs
	b.Write(chnaRow(1, "ATU_00000001", "AT_00010001_01", "AP_00010002"))
	b.Write(chnaRow(2, "ATU_00000002", "AT_00030001_01", "AP_00030001"))
	return b.Bytes()
}
