package review

// Region is a normalized rectangle in the video frame. Coordinates and sizes
// are in the inclusive range [0, 1].
type Region struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Cue is one subtitle event. EndMs is exclusive.
type Cue struct {
	ID      string  `json:"id"`
	StartMs int64   `json:"startMs"`
	EndMs   int64   `json:"endMs"`
	Text    string  `json:"text"`
	Region  *Region `json:"region,omitempty"`
}

// Track describes one subtitle source and its default display region.
type Track struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Source   string  `json:"source"`
	Language string  `json:"language,omitempty"`
	Region   *Region `json:"region,omitempty"`
	Cues     []Cue   `json:"-"`
}

type Config struct {
	FPS                  float64 `json:"fps"`
	MinOverlapMs         int64   `json:"minOverlapMs"`
	CandidateThreshold   float64 `json:"candidateThreshold"`
	MinTextSimilarity    float64 `json:"minTextSimilarity"`
	ManualReviewBelow    float64 `json:"manualReviewBelow"`
	ManualRegionBelow    float64 `json:"manualRegionBelow"`
	ManualTextBelow      float64 `json:"manualTextBelow"`
	ManualShortTextRunes int     `json:"manualShortTextRunes"`
	MinRegionOverlap     float64 `json:"minRegionOverlap"`
	Tracks               []Track `json:"tracks"`
}

type Thresholds struct {
	FPS                  float64 `json:"fps"`
	MinOverlapMs         int64   `json:"minOverlapMs"`
	CandidateThreshold   float64 `json:"candidateThreshold"`
	MinTextSimilarity    float64 `json:"minTextSimilarity"`
	ManualReviewBelow    float64 `json:"manualReviewBelow"`
	ManualRegionBelow    float64 `json:"manualRegionBelow"`
	ManualTextBelow      float64 `json:"manualTextBelow"`
	ManualShortTextRunes int     `json:"manualShortTextRunes"`
	MinRegionOverlap     float64 `json:"minRegionOverlap"`
}

type Report struct {
	SchemaVersion     string     `json:"schemaVersion"`
	Thresholds        Thresholds `json:"thresholds"`
	ConflictCount     int        `json:"conflictCount"`
	ManualReviewCount int        `json:"manualReviewCount"`
	Conflicts         []Conflict `json:"conflicts"`
}

type Conflict struct {
	ID                string             `json:"id"`
	TimeSegment       TimeSegment        `json:"timeSegment"`
	Region            RegionEvidence     `json:"region"`
	Tracks            TrackSources       `json:"tracks"`
	Text              TextEvidence       `json:"text"`
	Confidence        float64            `json:"confidence"`
	NeedsManualReview bool               `json:"needsManualReview"`
	ReviewReasons     []string           `json:"reviewReasons,omitempty"`
	FrameEvidence     FrameEvidence      `json:"frameEvidence"`
	RepairSuggestions []RepairSuggestion `json:"repairSuggestions"`
}

type TimeSegment struct {
	StartMs        int64  `json:"startMs"`
	EndMsExclusive int64  `json:"endMsExclusive"`
	StartTimecode  string `json:"startTimecode"`
	EndTimecode    string `json:"endTimecode"`
}

type RegionEvidence struct {
	ForcedRegion      Region  `json:"forcedRegion"`
	FullRegion        Region  `json:"fullRegion"`
	Intersection      Region  `json:"intersection"`
	IntersectionRatio float64 `json:"intersectionRatio"`
	Score             float64 `json:"score"`
}

type TrackSources struct {
	Forced TrackRef `json:"forced"`
	Full   TrackRef `json:"full"`
}

type TrackRef struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	Language string `json:"language,omitempty"`
	CueID    string `json:"cueId"`
}

type TextEvidence struct {
	ForcedText          string  `json:"forcedText"`
	FullText            string  `json:"fullText"`
	NormalizedForced    string  `json:"normalizedForced"`
	NormalizedFull      string  `json:"normalizedFull"`
	EditSimilarity      float64 `json:"editSimilarity"`
	ContainmentScore    float64 `json:"containmentScore"`
	BigramDice          float64 `json:"bigramDice"`
	Similarity          float64 `json:"similarity"`
	EffectiveSimilarity float64 `json:"effectiveSimilarity"`
	ShortTextAdjusted   bool    `json:"shortTextAdjusted"`
	ShortestTextRunes   int     `json:"shortestTextRunes"`
}

type FrameEvidence struct {
	FPS         float64      `json:"fps"`
	Overlap     FrameSpan    `json:"overlap"`
	ForcedCue   FrameCue     `json:"forcedCue"`
	FullCue     FrameCue     `json:"fullCue"`
	FrameChecks []FrameCheck `json:"frameChecks"`
}

type FrameSpan struct {
	StartFrame        int64 `json:"startFrame"`
	EndFrameExclusive int64 `json:"endFrameExclusive"`
	FrameCount        int64 `json:"frameCount"`
}

type FrameCue struct {
	CueID             string `json:"cueId"`
	StartFrame        int64  `json:"startFrame"`
	EndFrameExclusive int64  `json:"endFrameExclusive"`
}

type FrameCheck struct {
	Frame        int64 `json:"frame"`
	ForcedActive bool  `json:"forcedActive"`
	FullActive   bool  `json:"fullActive"`
}

type RepairSuggestion struct {
	Priority          int      `json:"priority"`
	Action            string   `json:"action"`
	TrackID           string   `json:"trackId"`
	CueIDs            []string `json:"cueIds"`
	StartFrame        int64    `json:"startFrame"`
	EndFrameExclusive int64    `json:"endFrameExclusive"`
	StartTimecode     string   `json:"startTimecode"`
	EndTimecode       string   `json:"endTimecode"`
	Automatic         bool     `json:"automatic"`
	Rationale         string   `json:"rationale"`
}
