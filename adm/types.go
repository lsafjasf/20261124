package adm

// ADM (ITU-R BS.2076) model, reduced to what the timeline analysis needs.

type Position struct {
	Azimuth   float64 // degrees, 0 = front, +left
	Elevation float64 // degrees, 0 = screen plane, +up
	Distance  float64 // 1.0 = speaker ring
	// Cartesian carries the raw X/Y/Z when the block is cartesian.
	Cartesian bool
	X, Y, Z   float64
}

type Divergence struct {
	Value         float64
	AzimuthRange  float64
	PositionRange float64
}

type Block struct {
	Rtime          float64 // seconds
	Duration       float64 // seconds; 0 means "until next block / open ended"
	Pos            Position
	Gain           float64 // linear, default 1
	Width          float64
	Height         float64
	Depth          float64
	Diffuse        float64
	ScreenEdgeLock bool
	Divergence     Divergence
	HasPosition    bool
}

type ChannelFormat struct {
	ID           string
	Name         string
	Type         string // "Objects", "DirectSpeakers", ...
	Cartesian    bool   // coordinateType="cartesian"
	SpeakerLabel string
	Blocks       []Block
}

type PackFormat struct {
	ID          string
	Name        string
	Type        string
	ChannelRefs []string
}

type Object struct {
	ID        string
	Name      string
	PackRefs  []string
	TrackRefs []string
	Gain      float64 // linear, default 1
	Dialogue  int     // -1 = unknown
}

type TrackUID struct {
	ID       string
	TrackIdx int
	TrackFmt string
}

type Document struct {
	Objects        []Object
	ChannelFormats map[string]*ChannelFormat
	PackFormats    map[string]*PackFormat
	TrackUIDs      map[string]*TrackUID
	Warnings       []string
}
