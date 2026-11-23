package stl

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Issue categories make the different failure classes independently consumable.
const (
	CategoryStructure  = "structure"
	CategoryGSI        = "gsi"
	CategoryFormat     = "format"
	CategoryDisplay    = "display_mode"
	CategoryTimecode   = "timecode"
	CategoryLanguage   = "language"
	CategoryBlockNo    = "block_number"
	CategoryCount      = "subtitle_count"
	CategoryEmpty      = "empty_subtitle"
	CategoryEmptyBlock = "empty_block"
	CategoryDuplicate  = "duplicate_block"
	CategoryOverlong   = "overlong_text"
	CategoryText       = "text_encoding"
)

// Severity levels.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// BlockRef identifies one TTI block in file order. Index is zero-based.
type BlockRef struct {
	Index  int    `json:"tti_index"`
	Number int    `json:"tti_number"`
	Offset int64  `json:"file_offset"`
	SGN    uint8  `json:"sgn"`
	SN     uint16 `json:"sn"`
	EBN    uint8  `json:"ebn"`
}

// Issue is either a GSI finding (Block is nil) or a TTI finding.
type Issue struct {
	Block          *BlockRef `json:"block,omitempty"`
	Category       string    `json:"category"`
	Severity       string    `json:"severity"`
	Field          string    `json:"field,omitempty"`
	Message        string    `json:"message"`
	Suggestion     string    `json:"suggestion"`
	RelatedIndexes []int     `json:"related_tti_indexes,omitempty"`
}

// Declared contains parsed GSI counts. A nil value means the field was absent
// or was not a valid decimal number.
type Declared struct {
	TNB *int `json:"tnb"`
	TNS *int `json:"tns"`
	TNG *int `json:"tng"`
	MNC *int `json:"mnc"`
	MNR *int `json:"mnr"`
}

// Actual contains counts derived from the TTI blocks in this file.
type Actual struct {
	TTIBlocks         int `json:"tti_blocks"`
	Subtitles         int `json:"subtitles"`
	NonEmptySubtitles int `json:"non_empty_subtitles"`
	SubtitleGroups    int `json:"subtitle_groups"`
	EmptySubtitles    int `json:"empty_subtitles"`
	DuplicateBlocks   int `json:"duplicate_blocks"`
}

// GSIInfo is the report-facing interpretation of the GSI block.
type GSIInfo struct {
	CPN          string `json:"cpn"`
	DFC          string `json:"dfc"`
	FrameRate    int    `json:"frame_rate"`
	DSC          string `json:"dsc"`
	DisplayMode  string `json:"display_mode,omitempty"`
	CCT          string `json:"cct"`
	LanguageCode string `json:"language_code"`
	Language     string `json:"language,omitempty"`
	TCS          string `json:"tcs"`
	TCP          string `json:"tcp"`
	TCF          string `json:"tcf"`
	TND          string `json:"tnd"`
	DSN          string `json:"dsn"`
	Programme    string `json:"programme,omitempty"`
	Episode      string `json:"episode,omitempty"`
}

// Summary counts findings by severity.
type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// Report is the complete, read-only result for one STL file.
type Report struct {
	File          string   `json:"file"`
	SizeBytes     int      `json:"size_bytes"`
	GSIComplete   bool     `json:"gsi_complete"`
	GSI           GSIInfo  `json:"gsi"`
	Declared      Declared `json:"declared"`
	Actual        Actual   `json:"actual"`
	TrailingBytes int      `json:"trailing_bytes"`
	Issues        []Issue  `json:"issues"`
	Summary       Summary  `json:"summary"`
}

// HasErrors reports whether any finding has error severity.
func (r Report) HasErrors() bool {
	return r.Summary.Errors > 0
}

// WriteJSON writes one report, or an array when reports contains more than one.
func WriteJSON(w io.Writer, reports []Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if len(reports) == 1 {
		return enc.Encode(reports[0])
	}
	return enc.Encode(reports)
}

// WriteText writes a compact human-readable report.
func WriteText(w io.Writer, reports []Report) error {
	for i, r := range reports {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := writeTextReport(w, r); err != nil {
			return err
		}
	}
	return nil
}

func writeTextReport(w io.Writer, r Report) error {
	if _, err := fmt.Fprintf(w, "%s: %d bytes\n", r.File, r.SizeBytes); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"GSI: DFC=%s fps=%s DSC=%s (%s) CCT=%s LC=%s (%s)\n",
		show(r.GSI.DFC), showInt(r.GSI.FrameRate), show(r.GSI.DSC),
		fallback(r.GSI.DisplayMode, "unknown"), show(r.GSI.CCT),
		show(r.GSI.LanguageCode), fallback(r.GSI.Language, "unknown")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"Declared: TNB=%s TNS=%s TNG=%s MNC=%s MNR=%s\n",
		showPtr(r.Declared.TNB), showPtr(r.Declared.TNS), showPtr(r.Declared.TNG),
		showPtr(r.Declared.MNC), showPtr(r.Declared.MNR)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"Actual: TTI=%d subtitles=%d non-empty=%d groups=%d empty=%d duplicates=%d\n",
		r.Actual.TTIBlocks, r.Actual.Subtitles, r.Actual.NonEmptySubtitles,
		r.Actual.SubtitleGroups, r.Actual.EmptySubtitles,
		r.Actual.DuplicateBlocks); err != nil {
		return err
	}
	for _, issue := range r.Issues {
		where := "GSI"
		if issue.Block != nil {
			where = fmt.Sprintf("TTI[%d] #%d (SGN=%d SN=%d EBN=0x%02X)",
				issue.Block.Index, issue.Block.Number, issue.Block.SGN,
				issue.Block.SN, issue.Block.EBN)
		}
		if _, err := fmt.Fprintf(w, "%s %s %s", strings.ToUpper(issue.Severity), where,
			issue.Category); err != nil {
			return err
		}
		if issue.Field != "" {
			if _, err := fmt.Fprintf(w, " %s", issue.Field); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, ": %s\n  suggestion: %s\n",
			issue.Message, issue.Suggestion); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "Summary: %d error(s), %d warning(s)\n",
		r.Summary.Errors, r.Summary.Warnings)
	return err
}

func blockRef(t TTI) *BlockRef {
	return &BlockRef{
		Index: t.Index, Number: t.Index + 1, Offset: t.Offset,
		SGN: t.SGN, SN: t.SN, EBN: t.EBN,
	}
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Block == nil && b.Block != nil {
			return true
		}
		if a.Block != nil && b.Block == nil {
			return false
		}
		if a.Block != nil && b.Block != nil && a.Block.Index != b.Block.Index {
			return a.Block.Index < b.Block.Index
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Field < b.Field
	})
}

func show(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func showInt(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func showPtr(n *int) string {
	if n == nil {
		return "invalid"
	}
	return fmt.Sprintf("%d", *n)
}

func fallback(s, other string) string {
	if s == "" {
		return other
	}
	return s
}
