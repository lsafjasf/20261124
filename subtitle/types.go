package subtitle

import "time"

// Kind 表示字幕轨道的类型。
type Kind string

const (
	// KindForced 强制叙述轨（Forced Narrative）：只翻译外语台词与屏幕文字，
	// 即使主字幕关闭也应在需要时显示。
	KindForced Kind = "forced"
	// KindFull 完整字幕轨（Full Subtitles）：覆盖全部对白。
	KindFull Kind = "full"
	// KindSDH 聋人与听障用户字幕轨（Subtitles for the Deaf and Hard-of-hearing），
	// 完整对白外加说话人、音效等描述。
	KindSDH Kind = "sdh"
)

// Cue 是一条字幕提示。Start/End 为相对于当前播放内容的时间。
type Cue struct {
	Start time.Duration
	End   time.Duration
	// Lines 为预排版后的文本行；WidthCells 是每行占用的单元格数
	// （东亚宽字符计 2 格，其余计 1 格）。
	Lines      []string
	WidthCells int
}

// Track 描述一条可选字幕轨。
type Track struct {
	ID       string
	Kind     Kind
	Language string // BCP-47 语言标签，例如 "en"、"zh-Hans"
	// AudioLanguage 为该强制轨所配套的音轨语言；
	// 仅 Kind == KindForced 时有意义。
	AudioLanguage string
	Cues          []Cue
}

// Preferences 是用户对字幕的偏好设置。
type Preferences struct {
	// SubtitlesEnabled 是主字幕总开关。
	// 关闭时常规字幕不显示，但强制叙述轨仍可能被选中。
	SubtitlesEnabled bool
	// PreferredLanguages 按优先级排列的字幕语言。
	PreferredLanguages []string
	// SDHMode 表示用户希望获得 SDH（听障辅助）字幕。
	SDHMode bool
}

// PlaybackState 是决策时刻的播放环境。
type PlaybackState struct {
	// AudioLanguage 为当前音轨的语言（BCP-47）。
	AudioLanguage string
	// At 为当前播放位置。
	At time.Duration
	// Readable 为当前窗口/视口中字幕可用的可读空间。
	Readable ReadableSpace
}

// ReadableSpace 描述字幕渲染区域的可读空间，以字符单元格为单位。
type ReadableSpace struct {
	// WidthCells 每行可容纳的单元格数。
	WidthCells int
	// HeightLines 可容纳的文本行数。
	HeightLines int
}

// Reason 解释最终选择（或不选择）某条轨道的原因。
type Reason string

const (
	ReasonNone              Reason = "none"
	ReasonSDHPreferred      Reason = "sdh_preferred"
	ReasonPreferredLanguage Reason = "preferred_language"
	ReasonLanguageFallback  Reason = "language_fallback"
	ReasonForcedOnly        Reason = "forced_only"
	ReasonForcedOverride    Reason = "forced_override"
)

// Decision 是一次显示决策的结果。
type Decision struct {
	// Track 为被选中的轨道；nil 表示当前时刻不显示任何字幕。
	Track *Track
	// Cue 为被选中轨道在当前时刻命中的提示；未命中时为 nil。
	Cue *Cue
	// Kind 为选中轨道的类型；未选中时为空字符串。
	Kind Kind
	// Fits 表示命中的提示在当前可读空间内可完整显示。
	// 强制轨即使放不下也会被选中，此时 Fits == false。
	Fits bool
	// Overflow 为提示超出可读空间的程度：正数表示多出的行数。
	Overflow int
	// Reason 为决策原因。
	Reason Reason
}
