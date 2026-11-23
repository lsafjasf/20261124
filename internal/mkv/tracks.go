package mkv

import (
	"fmt"
	"io"
)

func parseTracks(payload []byte, info *SourceInfo) error {
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if id != IDTrackEntry {
			continue
		}
		track, err := parseTrackEntry(p)
		if err != nil {
			return err
		}
		info.Tracks = append(info.Tracks, track)
	}
}

func parseTrackEntry(payload []byte) (*Track, error) {
	t := &Track{}
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch id {
		case IDTrackNumber:
			t.Number = uintOf(p)
		case IDTrackUID:
			t.UID = uintOf(p)
		case IDTrackType:
			t.Type = uintOf(p)
		case IDCodecID:
			t.CodecID = string(p)
		case IDCodecPrivate:
			t.CodecPrivate = append([]byte(nil), p...)
		case IDTrackName:
			t.Name = string(p)
		case IDTrackLanguage:
			t.Language = string(p)
		case IDFlagDefault:
			t.FlagDefault = uintOf(p) != 0
		case IDVideo:
			if err := parseVideo(p, t); err != nil {
				return nil, err
			}
		case IDAudio:
			if err := parseAudio(p, t); err != nil {
				return nil, err
			}
		}
	}
	if t.Number == 0 || t.CodecID == "" {
		return nil, fmt.Errorf("mkv: 轨道缺少 TrackNumber 或 CodecID,容器与码流无法同步")
	}
	return t, nil
}

func parseVideo(payload []byte, t *Track) error {
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch id {
		case IDPixelWidth:
			t.PixelWidth = uintOf(p)
		case IDPixelHeight:
			t.PixelHeight = uintOf(p)
		case IDColour:
			colour, err := parseColour(p)
			if err != nil {
				return err
			}
			t.Colour = colour
		}
	}
}

func parseAudio(payload []byte, t *Track) error {
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch id {
		case IDSamplingFreq:
			f, err := floatOf(p)
			if err != nil {
				return err
			}
			t.SamplingFreq = f
		case IDChannels:
			t.Channels = uintOf(p)
		case IDBitDepth:
			t.BitDepth = uintOf(p)
			t.HasBitDepth = true
		}
	}
}

func parseColour(payload []byte) (*Colour, error) {
	col := &Colour{}
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch id {
		case IDMatrixCoefficients:
			col.Matrix = uintOf(p)
			col.HasMatrix = true
		case IDRange:
			col.Range = uintOf(p)
			col.HasRange = true
		case IDTransferCharac:
			col.Transfer = uintOf(p)
			col.HasTransfer = true
		case IDPrimaries:
			col.Primaries = uintOf(p)
			col.HasPrimaries = true
		case IDMaxCLL:
			col.MaxCLL = uintOf(p)
			col.HasMaxCLL = true
		case IDMaxFALL:
			col.MaxFALL = uintOf(p)
			col.HasMaxFALL = true
		case IDMasteringMeta:
			if err := parseMastering(p, col); err != nil {
				return nil, err
			}
		}
	}
	return col, nil
}

func parseMastering(payload []byte, col *Colour) error {
	c := &cursor{data: payload}
	for {
		id, p, err := c.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		f, err := floatOf(p)
		if err != nil {
			return err
		}
		switch id {
		case IDPrimaryRX:
			col.Mastering.PrimaryRX = f
		case IDPrimaryRY:
			col.Mastering.PrimaryRY = f
		case IDPrimaryGX:
			col.Mastering.PrimaryGX = f
		case IDPrimaryGY:
			col.Mastering.PrimaryGY = f
		case IDPrimaryBX:
			col.Mastering.PrimaryBX = f
		case IDPrimaryBY:
			col.Mastering.PrimaryBY = f
		case IDWhitePointX:
			col.Mastering.WhitePointX = f
		case IDWhitePointY:
			col.Mastering.WhitePointY = f
		case IDLuminanceMax:
			col.Mastering.LuminanceMax = f
		case IDLuminanceMin:
			col.Mastering.LuminanceMin = f
		default:
			continue
		}
		col.MasteringFields++
	}
}
