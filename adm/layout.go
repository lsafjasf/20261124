package adm

import "math"

// Speaker is one output channel of a reproduction layout.
type Speaker struct {
	Name      string  // e.g. "M+030", "LFE1"
	Short     string  // e.g. "L", "C", "LFE"
	Azimuth   float64 // degrees
	Elevation float64
	LFE       bool
}

// Layout is a named reproduction speaker layout (BS.2051 style).
type Layout struct {
	Name     string
	Speakers []Speaker
}

func speaker(name, short string, az, el float64) Speaker {
	return Speaker{Name: name, Short: short, Azimuth: az, Elevation: el, LFE: short == "LFE"}
}

var layouts = map[string]Layout{
	"0+2+0": {Name: "0+2+0", Speakers: []Speaker{
		speaker("M+030", "L", 30, 0),
		speaker("M-030", "R", -30, 0),
	}},
	"0+5+0": {Name: "0+5+0", Speakers: []Speaker{
		speaker("M+030", "L", 30, 0),
		speaker("M+000", "C", 0, 0),
		speaker("M-030", "R", -30, 0),
		speaker("M+110", "Ls", 110, 0),
		speaker("M-110", "Rs", -110, 0),
		speaker("LFE1", "LFE", 45, -30),
	}},
	"4+5+0": {Name: "4+5+0", Speakers: []Speaker{
		speaker("M+030", "L", 30, 0),
		speaker("M+000", "C", 0, 0),
		speaker("M-030", "R", -30, 0),
		speaker("M+110", "Ls", 110, 0),
		speaker("M-110", "Rs", -110, 0),
		speaker("U+045", "Tfl", 45, 45),
		speaker("U-045", "Tfr", -45, 45),
		speaker("U+135", "Tbl", 135, 45),
		speaker("U-135", "Tbr", -135, 45),
		speaker("LFE1", "LFE", 45, -30),
	}},
}

// GetLayout returns a layout by BS.2051 name ("0+2+0", "0+5+0", "4+5+0").
func GetLayout(name string) (Layout, bool) {
	l, ok := layouts[name]
	return l, ok
}

// LayoutNames lists the supported layout names.
func LayoutNames() []string {
	return []string{"0+2+0", "0+5+0", "4+5+0"}
}

// unitVec converts polar audio coordinates (azimuth left-positive,
// elevation up-positive) to a unit vector (x=front, y=left, z=up).
func unitVec(az, el float64) [3]float64 {
	azr := az * math.Pi / 180
	elr := el * math.Pi / 180
	return [3]float64{
		math.Cos(elr) * math.Cos(azr),
		math.Cos(elr) * math.Sin(azr),
		math.Sin(elr),
	}
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
