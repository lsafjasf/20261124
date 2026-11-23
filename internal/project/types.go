package project

// Project 是一份剪辑工程的交换格式（JSON）。
// 工程由媒体库、顶层轨道组成；轨道可以通过 Children 嵌套。
type Project struct {
	SchemaVersion string  `json:"schemaVersion"`
	Name          string  `json:"name"`
	Rate          float64 `json:"rate"`
	Media         []Media `json:"media"`
	Tracks        []Track `json:"tracks"`
}

// Media 描述媒体库中的一个素材。
type Media struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Checksum string  `json:"checksum"`
	Duration float64 `json:"duration"`
}

// Track 描述一条轨道；Children 非空时为容器轨道（嵌套轨道）。
type Track struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Muted    *bool   `json:"muted"`
	Locked   *bool   `json:"locked"`
	Clips    []Clip  `json:"clips"`
	Children []Track `json:"children"`
}

// Clip 描述轨道上的一个片段。
type Clip struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	MediaID    string      `json:"mediaId"`
	TrackID    string      `json:"trackId"`
	TimelineIn float64     `json:"timelineIn"`
	SourceIn   float64     `json:"sourceIn"`
	SourceOut  float64     `json:"sourceOut"`
	Speed      float64     `json:"speed"`
	Enabled    *bool       `json:"enabled"`
	Transition *Transition `json:"transition"`
	Effects    []Effect    `json:"effects"`
}

// Transition 描述片段头部或尾部的转场。
type Transition struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Duration float64            `json:"duration"`
	At       string             `json:"at"`
	Params   map[string]float64 `json:"params"`
}

// Effect 描述挂载在片段上的一个效果。
type Effect struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Enabled *bool              `json:"enabled"`
	Params  map[string]float64 `json:"params"`
}

// FlatTrack 是展开嵌套结构后的轨道记录。
type FlatTrack struct {
	Track Track
	Path  []string // 从根轨道到当前轨道的 ID 路径
	Depth int
}

// FlatClip 是展开嵌套结构后的片段记录。
type FlatClip struct {
	Clip      Clip
	TrackPath []string // 片段所在轨道的 ID 路径
}

// Issue 是单份工程自身的结构性问题。
type Issue struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Track  string `json:"track,omitempty"`
	Detail string `json:"detail"`
}
