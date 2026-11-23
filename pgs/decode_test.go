package pgs

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSplitObjectSegments(t *testing.T) {
	pixels := make([]byte, 40*10)
	for y := 0; y < 10; y++ {
		for x := 0; x < 40; x++ {
			if x < 20 {
				pixels[y*40+x] = 1
			} else {
				pixels[y*40+x] = 2
			}
		}
	}
	rle := encodeTestRLE(pixels, 40, 10)
	split := len(rle) / 2
	first := splitODS(7, 0x80, 40, 10, 4+len(rle), rle[:split])
	second := splitODS(7, 0x40, 0, 0, 4+len(rle), rle[split:])

	d := newDecoder()
	if err := d.applyObject(first); err != nil {
		t.Fatal(err)
	}
	if err := d.applyObject(second); err != nil {
		t.Fatal(err)
	}
	obj := d.objects[7]
	if obj == nil || !obj.complete {
		t.Fatal("split object was not assembled")
	}
	if !bytes.Equal(obj.pixels, pixels) {
		t.Fatal("decoded split object differs from source bitmap")
	}
}

func TestRLEAllowsMissingFinalEndOfLine(t *testing.T) {
	pixels := solidPixels(5, 2, 3)
	rle := encodeTestRLE(pixels, 5, 2)
	rle = rle[:len(rle)-2]
	got, err := decodeRLE(rle, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pixels) {
		t.Fatalf("decoded pixels = %v", got)
	}
}

func splitODS(id uint16, flags byte, width, height, totalLen int, data []byte) []byte {
	p := make([]byte, 7)
	binary.BigEndian.PutUint16(p[0:2], id)
	p[3] = flags
	p[4] = byte(totalLen >> 16)
	p[5] = byte(totalLen >> 8)
	p[6] = byte(totalLen)
	if flags&0x80 != 0 {
		dim := make([]byte, 4)
		binary.BigEndian.PutUint16(dim[0:2], uint16(width))
		binary.BigEndian.PutUint16(dim[2:4], uint16(height))
		p = append(p, dim...)
	}
	return append(p, data...)
}
