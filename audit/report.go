package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"hdr10audit/hdr"
	"hdr10audit/mp4"
)

type FileReport struct {
	Path             string        `json:"path"`
	Bytes            int64         `json:"bytes"`
	SHA256           string        `json:"sha256"`
	MediaPayloadHash string        `json:"media_payload_sha256"`
	Precedence       string        `json:"precedence"`
	PrecedenceReason string        `json:"precedence_reason"`
	Tracks           []TrackReport `json:"tracks"`
	Issues           []hdr.Issue   `json:"issues,omitempty"`
	OK               bool          `json:"ok"`
	Summary          ReportSummary `json:"summary"`
}

type ReportSummary struct {
	TrackCount    int `json:"track_count"`
	ConflictCount int `json:"conflict_count"`
	ErrorCount    int `json:"error_count"`
	WarningCount  int `json:"warning_count"`
}

type TrackReport struct {
	ID        uint32            `json:"track_id"`
	Codec     string            `json:"codec"`
	Container SourceReport      `json:"container"`
	Bitstream SourceReport      `json:"bitstream"`
	Effective EffectiveMetadata `json:"effective"`
	Conflicts []Conflict        `json:"conflicts,omitempty"`
	Issues    []hdr.Issue       `json:"issues,omitempty"`
}

type SourceReport struct {
	Locations []OccurrenceView `json:"occurrences,omitempty"`
}

type OccurrenceView struct {
	Location  string            `json:"location"`
	Count     int               `json:"count,omitempty"`
	Mastering *MasteringView    `json:"mastering_display,omitempty"`
	Light     *ContentLightView `json:"content_light,omitempty"`
}

type EffectiveMetadata struct {
	MasteringSource string            `json:"mastering_source,omitempty"`
	LightsSource    string            `json:"content_light_source,omitempty"`
	Mastering       *MasteringView    `json:"mastering_display,omitempty"`
	Light           *ContentLightView `json:"content_light,omitempty"`
}

type Conflict struct {
	Field     string `json:"field"`
	Container string `json:"container"`
	Bitstream string `json:"bitstream"`
	Winner    string `json:"winner"`
	Reason    string `json:"reason"`
}

type MasteringView struct {
	Primaries     map[string]PointView `json:"primaries"`
	WhitePoint    PointView            `json:"white_point"`
	LuminanceNits LuminanceView        `json:"luminance_cd_m2"`
}

type PointView struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	XRaw uint16  `json:"x_raw"`
	YRaw uint16  `json:"y_raw"`
}

type LuminanceView struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	MinRaw uint32  `json:"min_raw"`
	MaxRaw uint32  `json:"max_raw"`
}

type ContentLightView struct {
	MaxCLL  uint16 `json:"max_cll"`
	MaxFALL uint16 `json:"max_fall"`
}

func Analyze(path string) (*FileReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	mf := mp4.NewFile(f, st.Size())
	info, err := mf.Inspect()
	if err != nil {
		return nil, err
	}
	fileHash, err := hashReader(f)
	if err != nil {
		return nil, err
	}
	mediaHash, err := hashRanges(f, info.MediaData)
	if err != nil {
		return nil, err
	}
	report := &FileReport{
		Path: path, Bytes: st.Size(), SHA256: fileHash,
		MediaPayloadHash: mediaHash,
		Precedence:       "bitstream_over_container",
		PrecedenceReason: precedenceReason,
	}
	report.Issues = append(report.Issues, info.Issues...)
	for _, track := range info.Tracks {
		tr := analyzeTrack(track)
		report.Tracks = append(report.Tracks, tr)
	}
	report.Summary.TrackCount = len(report.Tracks)
	for _, tr := range report.Tracks {
		report.Summary.ConflictCount += len(tr.Conflicts)
		countIssues(tr.Issues, &report.Summary)
	}
	countIssues(report.Issues, &report.Summary)
	report.OK = report.Summary.ErrorCount == 0
	return report, nil
}

func analyzeTrack(track mp4.Track) TrackReport {
	tr := TrackReport{ID: track.ID, Codec: track.Codec}
	tr.Issues = append(tr.Issues, track.Issues...)
	for _, occ := range track.Container {
		tr.Container.Locations = append(tr.Container.Locations, viewOccurrence(occ))
		tr.Issues = append(tr.Issues, hdr.Validate(occ.Metadata, occ.Source, occ.Location)...)
	}
	for _, occ := range track.Bitstream {
		tr.Bitstream.Locations = append(tr.Bitstream.Locations, viewOccurrence(occ))
		tr.Issues = append(tr.Issues, hdr.Validate(occ.Metadata, occ.Source, occ.Location)...)
	}
	container, ci := resolveSource(track.Container, hdr.SourceContainer)
	stream, si := resolveSource(track.Bitstream, hdr.SourceBitstream)
	tr.Issues = append(tr.Issues, ci...)
	tr.Issues = append(tr.Issues, si...)

	mastering, masteringSource, issues := chooseMastering(container, stream)
	if mastering != nil {
		tr.Effective.Mastering = viewMastering(*mastering)
		tr.Effective.MasteringSource = masteringSource
	}
	tr.Issues = append(tr.Issues, issues...)
	light, lightSource, issues := chooseLight(container, stream)
	if light != nil {
		tr.Effective.Light = viewContentLight(*light)
		tr.Effective.LightsSource = lightSource
	}
	tr.Issues = append(tr.Issues, issues...)
	tr.Conflicts = compareSources(container, stream)
	for _, conflict := range tr.Conflicts {
		tr.Issues = append(tr.Issues, hdr.Issue{
			Code: "metadata_conflict", Severity: hdr.SeverityError,
			Field: conflict.Field,
			Message: fmt.Sprintf("容器值 %s 与码流值 %s 冲突；以 %s 为准",
				conflict.Container, conflict.Bitstream, conflict.Winner),
		})
	}
	tr.Issues = append(tr.Issues, missingIssues(container, stream)...)
	return tr
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashRanges(r io.ReaderAt, ranges []mp4.Range) (string, error) {
	h := sha256.New()
	for _, rg := range ranges {
		if _, err := io.Copy(h, io.NewSectionReader(r, rg.Start, rg.Size)); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func countIssues(issues []hdr.Issue, sum *ReportSummary) {
	for _, issue := range issues {
		switch issue.Severity {
		case hdr.SeverityError:
			sum.ErrorCount++
		case hdr.SeverityWarning:
			sum.WarningCount++
		}
	}
}

func viewOccurrence(occ hdr.Occurrence) OccurrenceView {
	v := OccurrenceView{Location: occ.Location, Count: occ.Count}
	if occ.Metadata.Mastering != nil {
		v.Mastering = viewMastering(*occ.Metadata.Mastering)
	}
	if occ.Metadata.Light != nil {
		v.Light = viewContentLight(*occ.Metadata.Light)
	}
	return v
}

func viewMastering(m hdr.Mastering) *MasteringView {
	primaries := make(map[string]PointView, 3)
	for i, name := range []string{"G", "B", "R"} {
		primaries[name] = viewPoint(m.Primaries[i])
	}
	return &MasteringView{
		Primaries:  primaries,
		WhitePoint: viewPoint(m.WhitePoint),
		LuminanceNits: LuminanceView{
			Min: m.MinNits(), Max: m.MaxNits(),
			MinRaw: m.MinLuminance, MaxRaw: m.MaxLuminance,
		},
	}
}

func viewPoint(p hdr.Point) PointView {
	return PointView{X: p.XValue(), Y: p.YValue(), XRaw: p.X, YRaw: p.Y}
}

func viewContentLight(c hdr.ContentLight) *ContentLightView {
	return &ContentLightView{MaxCLL: c.MaxCLL, MaxFALL: c.MaxFALL}
}

func formatMastering(m *hdr.Mastering) string {
	if m == nil {
		return "missing"
	}
	return fmt.Sprintf("%s WP(%.5f,%.5f) L[%.4f,%.4f]",
		m.Primaries.String(), m.WhitePoint.XValue(), m.WhitePoint.YValue(),
		m.MinNits(), m.MaxNits())
}

func formatLight(c *hdr.ContentLight) string {
	if c == nil {
		return "missing"
	}
	return fmt.Sprintf("MaxCLL=%d MaxFALL=%d", c.MaxCLL, c.MaxFALL)
}

func sourceName(s hdr.Source) string {
	if s == "" {
		return "none"
	}
	return string(s)
}

func joinLocations(views []OccurrenceView) string {
	parts := make([]string, 0, len(views))
	for _, v := range views {
		parts = append(parts, v.Location)
	}
	return strings.Join(parts, ", ")
}
