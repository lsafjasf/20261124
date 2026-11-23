package diff

// FieldChange 是一个最小粒度的字段级变更。
type FieldChange struct {
	Path string      `json:"path"`
	From interface{} `json:"from"`
	To   interface{} `json:"to"`
}

// Change 是一条可执行的最小修改项。
// Op 取值：track.add / track.remove / track.modify /
//
//	clip.add / clip.remove / clip.modify / clip.move /
//	media.add / media.remove
type Change struct {
	Op        string        `json:"op"`
	ID        string        `json:"id"`
	Track     string        `json:"track,omitempty"`
	FromTrack string        `json:"fromTrack,omitempty"`
	ToTrack   string        `json:"toTrack,omitempty"`
	Detail    string        `json:"detail,omitempty"`
	Fields    []FieldChange `json:"fields,omitempty"`
}

// Conflict 是合并前必须人工裁决的冲突项。
// Kind 取值：
//
//	DUP_CLIP_ID        单份工程内片段 ID 重复
//	DUP_TRACK_ID       单份工程内轨道 ID 重复
//	NAME_MEDIA_CONFLICT 同名片段指向不同素材
//	MEDIA_CONTENT_CONFLICT 同素材 ID 的校验和/路径不一致
//	CLIP_UNKNOWN_MEDIA 片段引用了不存在的素材
//	BAD_SOURCE_RANGE   入出点非法
//	NESTED_TRACK       嵌套轨道相关问题（如重复挂载）
type Conflict struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Track  string `json:"track,omitempty"`
	Side   string `json:"side"` // base / target / both
	Detail string `json:"detail"`
}

// Report 是完整的可核查变更单。
type Report struct {
	Inputs    InputMeta  `json:"inputs"`
	Summary   Summary    `json:"summary"`
	Changes   []Change   `json:"changes"`
	Conflicts []Conflict `json:"conflicts"`
}

// InputMeta 记录输入指纹，保证变更单可复核。
type InputMeta struct {
	BasePath     string `json:"basePath"`
	BaseSHA256   string `json:"baseSha256"`
	TargetPath   string `json:"targetPath"`
	TargetSHA256 string `json:"targetSha256"`
}

// Summary 是变更与冲突计数。
type Summary struct {
	TracksAdded    int `json:"tracksAdded"`
	TracksRemoved  int `json:"tracksRemoved"`
	TracksModified int `json:"tracksModified"`
	ClipsAdded     int `json:"clipsAdded"`
	ClipsRemoved   int `json:"clipsRemoved"`
	ClipsModified  int `json:"clipsModified"`
	ClipsMoved     int `json:"clipsMoved"`
	MediaAdded     int `json:"mediaAdded"`
	MediaRemoved   int `json:"mediaRemoved"`
	Conflicts      int `json:"conflicts"`
}
