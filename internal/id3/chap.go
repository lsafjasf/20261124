package id3

func cstr(data []byte) (string, int) {
	for i, b := range data {
		if b == 0 {
			return string(data[:i]), i + 1
		}
	}
	return string(data), len(data)
}

func parseCHAP(f *Frame) {
	data := f.Data
	id, n := cstr(data)
	f.ElementID = id
	if n+16 > len(data) {
		return
	}
	f.Start = be32(data[n : n+4])
	f.End = be32(data[n+4 : n+8])
	f.StOff = be32(data[n+8 : n+12])
	f.EnOff = be32(data[n+12 : n+16])
	rest := data[n+16:]
	if len(rest) > 0 {
		subs, err := parseFrames(f.Version, rest)
		if err == nil {
			f.SubFrames = subs
			if t := subFrameTitle(subs); t != "" {
				f.Title = t
			}
		}
	}
}

func parseCTOC(ver Version, f *Frame) {
	data := f.Data
	id, n := cstr(data)
	f.ElementID = id
	if n+2 > len(data) {
		return
	}
	// CTOC flags byte, then entry count byte.
	count := int(data[n+1])
	pos := n + 2
	for i := 0; i < count && pos < len(data); i++ {
		child, used := cstr(data[pos:])
		f.ChildIDs = append(f.ChildIDs, child)
		pos += used
	}
	rest := data[pos:]
	if len(rest) > 0 {
		subs, err := parseFrames(ver, rest)
		if err == nil {
			f.SubFrames = subs
			if t := subFrameTitle(subs); t != "" {
				f.Title = t
			}
		}
	}
}

func subFrameTitle(subs []*Frame) string {
	for _, s := range subs {
		if s.ID == "TIT2" {
			return s.Text
		}
	}
	return ""
}
