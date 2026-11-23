package adm

import (
	"strconv"
	"strings"
)

func parseChannelFormat(n *xmlNode, doc *Document) *ChannelFormat {
	cf := &ChannelFormat{
		ID:   n.attr("audioChannelFormatID"),
		Name: n.attr("audioChannelFormatName"),
		Type: normalizeTypeLabel(n.attr("typeLabel"), n.attr("typeDefinition")),
	}
	if strings.EqualFold(n.attr("coordinateType"), "cartesian") {
		cf.Cartesian = true
	}
	for i := range n.Children {
		c := &n.Children[i]
		switch c.XMLName.Local {
		case "speakerLabel":
			cf.SpeakerLabel = strings.TrimSpace(c.Text)
		case "audioBlockFormat":
			cf.Blocks = append(cf.Blocks, parseBlock(c, cf, doc))
		}
	}
	return cf
}

// normalizeTypeLabel maps BS.2076 numeric type labels to type names.
func normalizeTypeLabel(label, definition string) string {
	if definition != "" {
		return definition
	}
	switch label {
	case "0001":
		return "DirectSpeakers"
	case "0002":
		return "Matrix"
	case "0003":
		return "Objects"
	case "0004":
		return "HOA"
	case "0005":
		return "Binaural"
	}
	return label
}

func parseBlock(n *xmlNode, cf *ChannelFormat, doc *Document) Block {
	b := Block{Gain: 1, Pos: Position{Distance: 1}}
	b.Rtime = parseClock(n.attr("rtime"))
	b.Duration = parseClock(n.attr("duration"))
	for _, c := range n.Children {
		switch c.XMLName.Local {
		case "position":
			coord := c.attr("coordinate")
			v, err := strconv.ParseFloat(strings.TrimSpace(c.Text), 64)
			if err != nil {
				continue
			}
			switch strings.ToLower(coord) {
			case "azimuth":
				b.Pos.Azimuth = v
				b.HasPosition = true
			case "elevation":
				b.Pos.Elevation = v
				b.HasPosition = true
			case "distance":
				b.Pos.Distance = v
			case "x":
				b.Pos.X = v
				b.Pos.Cartesian = true
				b.HasPosition = true
			case "y":
				b.Pos.Y = v
				b.Pos.Cartesian = true
				b.HasPosition = true
			case "z":
				b.Pos.Z = v
				b.Pos.Cartesian = true
				b.HasPosition = true
			}
		case "gain":
			if v, err := strconv.ParseFloat(strings.TrimSpace(c.Text), 64); err == nil {
				b.Gain = v
			}
		case "width":
			b.Width = floatVal(c.Text)
		case "height":
			b.Height = floatVal(c.Text)
		case "depth":
			b.Depth = floatVal(c.Text)
		case "diffuse":
			b.Diffuse = floatVal(c.Text)
		case "screenEdgeLock":
			b.ScreenEdgeLock = true
		case "divergence":
			b.Divergence.Value = floatVal(c.Text)
			if v := c.attr("azimuthRange"); v != "" {
				b.Divergence.AzimuthRange, _ = strconv.ParseFloat(v, 64)
			}
			if v := c.attr("positionRange"); v != "" {
				b.Divergence.PositionRange, _ = strconv.ParseFloat(v, 64)
			}
		}
	}
	if b.Pos.Cartesian != cf.Cartesian && b.HasPosition {
		doc.Warnings = append(doc.Warnings,
			"channelFormat "+cf.ID+": block coordinate type does not match channelFormat coordinateType")
	}
	return b
}

func floatVal(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

// parseClock accepts "SS", "SS.mmm" or "HH:MM:SS.mmm".
func parseClock(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if !strings.Contains(s, ":") {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	parts := strings.Split(s, ":")
	var sec float64
	mult := 1.0
	for i := len(parts) - 1; i >= 0; i-- {
		v, _ := strconv.ParseFloat(parts[i], 64)
		sec += v * mult
		mult *= 60
	}
	return sec
}
