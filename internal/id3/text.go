package id3

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// DecodeText decodes an ID3 text frame payload, skipping its encoding byte.
func DecodeText(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	enc := data[0]
	body := data[1:]
	switch enc {
	case 0: // ISO-8859-1
		var b strings.Builder
		for _, c := range body {
			b.WriteRune(rune(c))
		}
		return strings.TrimRight(b.String(), "\x00")
	case 1: // UTF-16 with BOM
		return decodeUTF16(body)
	case 2: // UTF-16BE without BOM
		return decodeUTF16Units(body, binary.BigEndian)
	case 3: // UTF-8
		return strings.TrimRight(string(body), "\x00")
	default:
		return strings.TrimRight(string(body), "\x00")
	}
}

func decodeUTF16(body []byte) string {
	if len(body) >= 2 && body[0] == 0xff && body[1] == 0xfe {
		return decodeUTF16Units(body[2:], binary.LittleEndian)
	}
	if len(body) >= 2 && body[0] == 0xfe && body[1] == 0xff {
		return decodeUTF16Units(body[2:], binary.BigEndian)
	}
	return decodeUTF16Units(body, binary.BigEndian)
}

func decodeUTF16Units(body []byte, order binary.ByteOrder) string {
	if len(body)%2 == 1 {
		body = body[:len(body)-1]
	}
	units := make([]uint16, 0, len(body)/2)
	for i := 0; i+1 < len(body); i += 2 {
		u := order.Uint16(body[i : i+2])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	runes := utf16.Decode(units)
	var b strings.Builder
	for _, r := range runes {
		if r == 0 {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// EncodeText encodes s using the given ID3 encoding byte (0..3).
func EncodeText(enc byte, s string) []byte {
	switch enc {
	case 3:
		out := make([]byte, 0, len(s)+2)
		out = append(out, 3)
		out = append(out, s...)
		out = append(out, 0)
		return out
	case 1, 2:
		return encodeUTF16(enc, s)
	default:
		out := make([]byte, 0, len(s)+2)
		out = append(out, 0)
		for _, r := range s {
			if r < 0x100 {
				out = append(out, byte(r))
			} else {
				out = append(out, '?')
			}
		}
		out = append(out, 0)
		return out
	}
}

func encodeUTF16(enc byte, s string) []byte {
	units := utf16.Encode([]rune(s))
	out := []byte{enc}
	if enc == 1 {
		out = append(out, 0xff, 0xfe) // BOM, then LE
	}
	for _, u := range units {
		if enc == 1 {
			out = append(out, byte(u), byte(u>>8))
		} else {
			out = append(out, byte(u>>8), byte(u))
		}
	}
	out = append(out, 0, 0)
	return out
}

// TLENMillis parses a TLEN frame value: either plain milliseconds or the
// hh:mm:ss.mmm form. Returns 0 when nothing usable is present.
func TLENMillis(text string) uint32 {
	t := strings.TrimSpace(text)
	if t == "" {
		return 0
	}
	if !strings.ContainsAny(t, ":x") {
		return parseUint32Digits(t)
	}
	parts := strings.FieldsFunc(t, func(r rune) bool { return r == ':' || r == '.' })
	var total uint32
	mult := []uint32{3600000, 60000, 1000, 1}
	for i, p := range parts {
		if i >= len(mult) {
			break
		}
		total += parseUint32Digits(p) * mult[i]
	}
	return total
}

func parseUint32Digits(s string) uint32 {
	var v uint32
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		v = v*10 + uint32(r-'0')
	}
	return v
}
