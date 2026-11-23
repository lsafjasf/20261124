package truehd

import "fmt"

const (
	trueHDStreamType = 0xba
	majorHeaderBase  = 28
	maxSubstreams    = 4
)

var majorMagic = [...]byte{0xf8, 0x72, 0x6f, 0xba}

type streamConfig struct {
	SampleRate      int
	SamplesPerFrame int
	Channels        int
	NumSubstreams   int
	MajorHeaderSize int
}

type substream struct {
	Start     int
	End       int
	CheckData bool
}

type frameHeader struct {
	Config    *streamConfig
	Major     bool
	HeaderLen int
	Subs      []substream
}

type parseFailure struct {
	truncated bool
	reason    string
}

func (e *parseFailure) Error() string {
	return e.reason
}

func auLength(data []byte, off int) (int, bool) {
	if off < 0 || off+2 > len(data) {
		return 0, false
	}
	n := (int(data[off])<<8 | int(data[off+1])) & 0x0fff
	return n * 2, true
}

func majorPrefixLen(data []byte, off int) int {
	if off < 0 || off >= len(data) {
		return 0
	}
	n := 0
	for n < len(majorMagic) && off+n < len(data) && data[off+n] == majorMagic[n] {
		n++
	}
	return n
}

func trueHDChannels(mask int) int {
	widths := [...]int{2, 1, 1, 2, 2, 2, 2, 1, 1, 2, 2, 1, 1}
	total := 0
	for i, width := range widths {
		if mask&(1<<i) != 0 {
			total += width
		}
	}
	return total
}

func parseMajorSync(data []byte, sync int) (*streamConfig, *parseFailure) {
	if sync < 0 || len(data)-sync < 4 {
		return nil, &parseFailure{truncated: true, reason: "truncated major-sync signature"}
	}
	if majorPrefixLen(data, sync) != len(majorMagic) {
		return nil, &parseFailure{reason: "bad major-sync signature"}
	}
	if len(data)-sync < majorHeaderBase {
		return nil, &parseFailure{truncated: true, reason: "truncated major-sync header"}
	}

	h := data[sync:]
	size := majorHeaderBase
	if h[25]&1 != 0 {
		extensions := int(h[26] >> 4)
		size += 2 + extensions*2
		if size > 64 {
			return nil, &parseFailure{reason: fmt.Sprintf("implausible major-sync header size %d", size)}
		}
		if len(h) < size {
			return nil, &parseFailure{truncated: true, reason: "truncated major-sync extension"}
		}
	}
	if h[3] != trueHDStreamType {
		return nil, &parseFailure{
			reason: fmt.Sprintf("unsupported stream type 0x%02x (want TrueHD 0xba)", h[3]),
		}
	}
	if !validMajorSyncChecksum(h[:size]) {
		return nil, &parseFailure{reason: "major-sync checksum mismatch"}
	}

	rateBits := int(h[4] >> 4)
	if rateBits == 0x0f {
		return nil, &parseFailure{reason: "unknown sample rate"}
	}
	rateFactor := rateBits & 7
	rate := 48000
	if rateBits&8 != 0 {
		rate = 44100
	}
	rate <<= rateFactor
	if rate <= 0 || rate > 192000 {
		return nil, &parseFailure{reason: fmt.Sprintf("unsupported sample rate %d", rate)}
	}

	numSubstreams := int(h[16] >> 4)
	if numSubstreams < 1 || numSubstreams > maxSubstreams {
		return nil, &parseFailure{
			reason: fmt.Sprintf("invalid substream count %d (must be 1..%d)", numSubstreams, maxSubstreams),
		}
	}

	arr1 := (int(h[5])&0x0f)<<1 | int(h[6])>>7
	arr2 := (int(h[6])&0x1f)<<8 | int(h[7])
	channels := trueHDChannels(arr1)
	if n := trueHDChannels(arr2); n > channels {
		channels = n
	}

	return &streamConfig{
		SampleRate:      rate,
		SamplesPerFrame: 40 << rateFactor,
		Channels:        channels,
		NumSubstreams:   numSubstreams,
		MajorHeaderSize: size,
	}, nil
}

func parseSubstreamHeaders(data []byte, packet, auLen, headerLen int, isMajor bool, cfg *streamConfig) ([]substream, *parseFailure) {
	subHeaderStart := packet + headerLen
	subHeaderBytes := 0
	prevEnd := 0
	subs := make([]substream, 0, cfg.NumSubstreams)

	for i := 0; i < cfg.NumSubstreams; i++ {
		off := subHeaderStart + subHeaderBytes
		if headerLen+subHeaderBytes+2 > auLen {
			return nil, &parseFailure{reason: fmt.Sprintf("access unit too short for substream %d header", i)}
		}
		if off+2 > len(data) {
			return nil, &parseFailure{truncated: true, reason: fmt.Sprintf("truncated substream %d header", i)}
		}

		flags := data[off]
		end := ((int(flags)&0x0f)<<8 | int(data[off+1])) * 2
		nonRestart := flags&0x40 != 0
		if nonRestart == isMajor {
			return nil, &parseFailure{
				reason: fmt.Sprintf("substream %d nonrestart flag conflicts with major-sync frame", i),
			}
		}

		subHeaderBytes += 2
		if flags&0x80 != 0 {
			if headerLen+subHeaderBytes+2 > auLen {
				return nil, &parseFailure{reason: fmt.Sprintf("access unit too short for substream %d extra word", i)}
			}
			if off+4 > len(data) {
				return nil, &parseFailure{truncated: true, reason: fmt.Sprintf("truncated substream %d extra word", i)}
			}
			subHeaderBytes += 2
		}

		if end < prevEnd {
			return nil, &parseFailure{reason: fmt.Sprintf("substream %d end offset moves backwards", i)}
		}
		capacity := auLen - headerLen - subHeaderBytes
		if end > capacity {
			return nil, &parseFailure{
				reason: fmt.Sprintf("substream %d end %d exceeds payload capacity %d", i, end, capacity),
			}
		}
		subs = append(subs, substream{Start: prevEnd, End: end, CheckData: flags&0x20 != 0})
		prevEnd = end
	}

	parity := xorBytes(data[packet : packet+4])
	parity ^= xorBytes(data[subHeaderStart : subHeaderStart+subHeaderBytes])
	if ((parity>>4)^parity)&0x0f != 0x0f {
		return nil, &parseFailure{reason: "access-unit/substream header parity mismatch"}
	}

	payloadBase := packet + headerLen + subHeaderBytes
	for i := range subs {
		subs[i].Start += payloadBase
		subs[i].End += payloadBase
	}
	return subs, nil
}

func inspectFrame(data []byte, packet int, cfg *streamConfig) (*frameHeader, *parseFailure) {
	if packet < 0 || packet+4 > len(data) {
		return nil, &parseFailure{truncated: true, reason: "truncated access-unit header"}
	}
	auLen, ok := auLength(data, packet)
	if !ok || auLen < 4 {
		return nil, &parseFailure{reason: "invalid access-unit length"}
	}

	sync := packet + 4
	prefix := majorPrefixLen(data, sync)
	isMajor := prefix == len(majorMagic)
	if prefix > 0 && !isMajor && packet+auLen > len(data) {
		return nil, &parseFailure{truncated: true, reason: "truncated major-sync signature"}
	}

	fh := &frameHeader{Major: isMajor, HeaderLen: 4}
	majorCfg := cfg
	if isMajor {
		var failure *parseFailure
		majorCfg, failure = parseMajorSync(data, sync)
		if failure != nil {
			return nil, failure
		}
		fh.Config = majorCfg
		fh.HeaderLen += majorCfg.MajorHeaderSize
	} else {
		if cfg == nil {
			return nil, &parseFailure{reason: "stream parameters missing; no valid major sync before this packet"}
		}
		fh.Config = cfg
	}

	subs, failure := parseSubstreamHeaders(data, packet, auLen, fh.HeaderLen, isMajor, fh.Config)
	if failure != nil {
		return nil, failure
	}
	fh.Subs = subs
	return fh, nil
}
