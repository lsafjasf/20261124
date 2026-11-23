package bitstream

import (
	"fmt"

	"hdr10audit/hdr"
)

type Codec int

const (
	HEVC Codec = iota + 1
	AVC
)

type ParseResult struct {
	Occurrences []hdr.Occurrence
	Issues      []hdr.Issue
}

func SplitNALUnits(data []byte, lengthSize int) ([][]byte, error) {
	if lengthSize != 1 && lengthSize != 2 && lengthSize != 3 && lengthSize != 4 {
		return nil, fmt.Errorf("invalid NAL length size %d", lengthSize)
	}
	var nals [][]byte
	for off := 0; off < len(data); {
		if len(data)-off < lengthSize {
			return nil, fmt.Errorf("truncated NAL length at sample offset %d", off)
		}
		n := 0
		for i := 0; i < lengthSize; i++ {
			n = n<<8 | int(data[off+i])
		}
		off += lengthSize
		if n < 0 || n > len(data)-off {
			return nil, fmt.Errorf("NAL length %d at sample offset %d exceeds remaining %d",
				n, off-lengthSize, len(data)-off)
		}
		if n == 0 {
			return nil, fmt.Errorf("zero-length NAL at sample offset %d", off-lengthSize)
		}
		nals = append(nals, data[off:off+n])
		off += n
	}
	return nals, nil
}

func ScanNAL(nal []byte, codec Codec, location string) ParseResult {
	var out ParseResult
	if len(nal) == 0 {
		return out
	}
	var nalType int
	switch codec {
	case HEVC:
		if len(nal) < 2 {
			return out
		}
		nalType = int(nal[0]>>1) & 0x3f
		if nalType != 39 && nalType != 40 {
			return out
		}
		nal = nal[2:]
	case AVC:
		nalType = int(nal[0] & 0x1f)
		if nalType != 6 {
			return out
		}
		nal = nal[1:]
	default:
		out.Issues = append(out.Issues, issue("unsupported_codec", "", location,
			"不支持的码流类型"))
		return out
	}
	return scanSEI(UnescapeRBSP(nal), codec, location)
}

func scanSEI(rbsp []byte, codec Codec, location string) ParseResult {
	var out ParseResult
	for off, msgIndex := 0, 0; off < len(rbsp); msgIndex++ {
		if isRBSPTrailing(rbsp[off:]) {
			break
		}
		payloadType, next, ok := readSEINumber(rbsp, off)
		if !ok {
			out.Issues = append(out.Issues, issue("malformed_sei", "", location,
				"SEI payload type 截断"))
			break
		}
		payloadSize, next2, ok := readSEINumber(rbsp, next)
		if !ok {
			out.Issues = append(out.Issues, issue("malformed_sei", "", location,
				"SEI payload size 截断"))
			break
		}
		if payloadSize > len(rbsp)-next2 {
			out.Issues = append(out.Issues, issue("malformed_sei", "", location,
				fmt.Sprintf("SEI payload %d 声明 %d 字节，仅剩 %d 字节",
					payloadType, payloadSize, len(rbsp)-next2)))
			break
		}
		payload := rbsp[next2 : next2+payloadSize]
		loc := fmt.Sprintf("%s/sei[%d]#%d", location, msgIndex, payloadType)
		switch {
		case payloadType == 137:
			addMastering(&out, payload, loc)
		case payloadType == 144:
			addLight(&out, payload, loc)
		case payloadType == 4:
			addT35(&out, payload, loc)
		}
		off = next2 + payloadSize
	}
	return out
}

func isRBSPTrailing(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if data[0] != 0x80 {
		return false
	}
	for _, b := range data[1:] {
		if b != 0 {
			return false
		}
	}
	return true
}

func addMastering(out *ParseResult, payload []byte, location string) {
	m, err := hdr.DecodeMastering(payload)
	if err != nil {
		out.Issues = append(out.Issues, issue("malformed_mdcv", "mastering", location, err.Error()))
		return
	}
	out.Occurrences = append(out.Occurrences, hdr.Occurrence{
		Source: hdr.SourceBitstream, Location: location,
		Metadata: hdr.Metadata{Mastering: &m},
	})
}

func addLight(out *ParseResult, payload []byte, location string) {
	c, err := hdr.DecodeContentLight(payload)
	if err != nil {
		out.Issues = append(out.Issues, issue("malformed_clli", "max_cll", location, err.Error()))
		return
	}
	out.Occurrences = append(out.Occurrences, hdr.Occurrence{
		Source: hdr.SourceBitstream, Location: location,
		Metadata: hdr.Metadata{Light: &c},
	})
}

func addT35(out *ParseResult, payload []byte, location string) {
	if len(payload) < 5 || payload[0] != 0xb5 ||
		payload[1] != 0x00 || payload[2] != 0x3c ||
		payload[3] != 0x00 || payload[4] != 0x01 {
		return
	}
	// HDR10 uses application_identifier 4 followed by application_version.
	// Some muxers omit the version byte, so accept both starts.
	for _, start := range []int{7, 6, 5} {
		remaining := payload[start:]
		switch len(remaining) {
		case 24:
			if m, err := hdr.DecodeMastering(remaining); err == nil {
				out.Occurrences = append(out.Occurrences, hdr.Occurrence{
					Source:   hdr.SourceBitstream,
					Location: location + "/t35",
					Metadata: hdr.Metadata{Mastering: &m},
				})
				return
			}
		case 28:
			if m, err := hdr.DecodeMastering(remaining[:24]); err == nil {
				c, _ := hdr.DecodeContentLight(remaining[24:])
				out.Occurrences = append(out.Occurrences, hdr.Occurrence{
					Source:   hdr.SourceBitstream,
					Location: location + "/t35",
					Metadata: hdr.Metadata{Mastering: &m, Light: &c},
				})
				return
			}
		}
	}
}

func readSEINumber(data []byte, off int) (int, int, bool) {
	n := 0
	for off < len(data) {
		b := data[off]
		off++
		n += int(b)
		if b != 0xff {
			return n, off, true
		}
	}
	return 0, off, false
}

func UnescapeRBSP(data []byte) []byte {
	out := make([]byte, 0, len(data))
	zeros := 0
	for i := 0; i < len(data); i++ {
		if zeros >= 2 && data[i] == 0x03 && i+1 < len(data) && data[i+1] <= 0x03 {
			zeros = 0
			continue
		}
		out = append(out, data[i])
		if data[i] == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func issue(code, field, location, msg string) hdr.Issue {
	return hdr.Issue{
		Code: code, Severity: hdr.SeverityError, Source: hdr.SourceBitstream,
		Field: field, Location: location, Message: msg,
	}
}
