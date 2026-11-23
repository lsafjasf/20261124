package hdr

import (
	"encoding/binary"
	"fmt"
)

func DecodeMastering(raw []byte) (Mastering, error) {
	if len(raw) != 24 {
		return Mastering{}, fmt.Errorf("mastering display payload is %d bytes, want 24", len(raw))
	}
	var m Mastering
	for i := range m.Primaries {
		m.Primaries[i].X = binary.BigEndian.Uint16(raw[i*4 : i*4+2])
		m.Primaries[i].Y = binary.BigEndian.Uint16(raw[i*4+2 : i*4+4])
	}
	m.WhitePoint.X = binary.BigEndian.Uint16(raw[12:14])
	m.WhitePoint.Y = binary.BigEndian.Uint16(raw[14:16])
	m.MaxLuminance = binary.BigEndian.Uint32(raw[16:20])
	m.MinLuminance = binary.BigEndian.Uint32(raw[20:24])
	return m, nil
}

func EncodeMastering(m Mastering) []byte {
	raw := make([]byte, 24)
	for i := range m.Primaries {
		binary.BigEndian.PutUint16(raw[i*4:], m.Primaries[i].X)
		binary.BigEndian.PutUint16(raw[i*4+2:], m.Primaries[i].Y)
	}
	binary.BigEndian.PutUint16(raw[12:], m.WhitePoint.X)
	binary.BigEndian.PutUint16(raw[14:], m.WhitePoint.Y)
	binary.BigEndian.PutUint32(raw[16:], m.MaxLuminance)
	binary.BigEndian.PutUint32(raw[20:], m.MinLuminance)
	return raw
}

func DecodeContentLight(raw []byte) (ContentLight, error) {
	if len(raw) != 4 {
		return ContentLight{}, fmt.Errorf("content light payload is %d bytes, want 4", len(raw))
	}
	return ContentLight{
		MaxCLL:  binary.BigEndian.Uint16(raw[0:2]),
		MaxFALL: binary.BigEndian.Uint16(raw[2:4]),
	}, nil
}

func EncodeContentLight(c ContentLight) []byte {
	raw := make([]byte, 4)
	binary.BigEndian.PutUint16(raw[0:], c.MaxCLL)
	binary.BigEndian.PutUint16(raw[2:], c.MaxFALL)
	return raw
}
