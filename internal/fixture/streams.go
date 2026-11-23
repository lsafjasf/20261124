package fixture

// Segment type bytes (mirror package pgs, kept local to avoid a cycle).
const (
	typePalette = 0x14
	typeObject  = 0x15
	typeDisplay = 0x16
	typeWindow  = 0x17
	typeClear   = 0x80
)

const epochStart = 0x80

var paletteV1 = Palette(0, 1,
	[5]byte{0, 16, 128, 128, 0},
	[5]byte{1, 235, 128, 128, 255},
)

var paletteV2 = Palette(0, 2,
	[5]byte{0, 16, 128, 128, 0},
	[5]byte{1, 81, 90, 240, 255},
)

var window0 = Window(Win{ID: 0, X: 100, Y: 800, W: 800, H: 120})

var object0 = Object(0, 1, 800, 120, []byte{0x00, 0x63, 0x01})

func defSet(pts uint32, palette []byte) []Seg {
	return []Seg{
		{Type: typePalette, PTS: pts, Payload: palette},
		{Type: typeWindow, PTS: pts, Payload: window0},
		{Type: typeObject, PTS: pts, Payload: object0},
	}
}

func displaySeg(state byte, pts uint32) Seg {
	return Seg{
		Type:    typeDisplay,
		PTS:     pts,
		Payload: Display(state, false, 0, CompObj{ObjectID: 0, WindowID: 0, X: 100, Y: 800}),
	}
}

func clearSeg(pts uint32) Seg { return Seg{Type: typeClear, PTS: pts} }

func concat(sets ...[]Seg) []Seg {
	var out []Seg
	for _, s := range sets {
		out = append(out, s...)
	}
	return out
}

// Streams returns the named binary fixtures written to testdata.
func Streams() map[string][]byte {
	clean := concat(
		defSet(90000, paletteV1),
		[]Seg{displaySeg(epochStart, 90000), clearSeg(135000)},
		[]Seg{displaySeg(0x00, 180000), clearSeg(225000)},
		defSet(270000, paletteV2),
		[]Seg{displaySeg(epochStart, 270000), clearSeg(315000)},
	)
	crossEpoch := concat(
		defSet(90000, paletteV1),
		[]Seg{displaySeg(epochStart, 90000), clearSeg(135000)},
		// Epoch 1 defines nothing and relies on epoch 0's palette,
		// window and object: the classic stale-state server bug.
		[]Seg{displaySeg(epochStart, 180000), clearSeg(225000)},
	)
	residual := concat(
		defSet(90000, paletteV1),
		// Epoch 0's composition is never cleared before the next
		// epoch start: the residual window case.
		[]Seg{displaySeg(epochStart, 90000)},
		[]Seg{displaySeg(epochStart, 180000), clearSeg(225000)},
	)
	return map[string][]byte{
		"clean_two_epochs.sup":    Build(clean...),
		"cross_epoch_palette.sup": Build(crossEpoch...),
		"residual_window.sup":     Build(residual...),
	}
}
