package audit

import (
	"fmt"
	"io"
	"strings"

	"hdr10audit/hdr"
)

func FormatFileReport(w io.Writer, r *FileReport) {
	fmt.Fprintf(w, "FILE %s\n", r.Path)
	fmt.Fprintf(w, "  size=%d sha256=%s\n", r.Bytes, r.SHA256)
	fmt.Fprintf(w, "  media_payload_sha256=%s\n", r.MediaPayloadHash)
	for _, issue := range r.Issues {
		formatIssue(w, issue)
	}
	for _, tr := range r.Tracks {
		formatTrack(w, tr)
	}
	fmt.Fprintf(w, "SUMMARY tracks=%d conflicts=%d errors=%d warnings=%d ok=%t\n",
		r.Summary.TrackCount, r.Summary.ConflictCount,
		r.Summary.ErrorCount, r.Summary.WarningCount, r.OK)
	fmt.Fprintf(w, "POLICY %s: %s\n", r.Precedence, r.PrecedenceReason)
}

func formatTrack(w io.Writer, tr TrackReport) {
	fmt.Fprintf(w, "  TRACK id=%d codec=%s\n", tr.ID, tr.Codec)
	formatSource(w, "container", tr.Container)
	formatSource(w, "bitstream", tr.Bitstream)
	if tr.Effective.Mastering != nil {
		fmt.Fprintf(w, "    effective.mastering[%s] %s\n",
			tr.Effective.MasteringSource, printMastering(tr.Effective.Mastering))
	}
	if tr.Effective.Light != nil {
		fmt.Fprintf(w, "    effective.content_light[%s] MaxCLL=%d MaxFALL=%d\n",
			tr.Effective.LightsSource,
			tr.Effective.Light.MaxCLL, tr.Effective.Light.MaxFALL)
	}
	for _, c := range tr.Conflicts {
		fmt.Fprintf(w, "    CONFLICT %s container=%s bitstream=%s winner=%s\n",
			c.Field, c.Container, c.Bitstream, c.Winner)
	}
	for _, issue := range tr.Issues {
		formatIssue(w, issue)
	}
}

func formatSource(w io.Writer, name string, source SourceReport) {
	if len(source.Locations) == 0 {
		fmt.Fprintf(w, "    %s: missing\n", name)
		return
	}
	for _, occ := range source.Locations {
		extra := ""
		if occ.Count > 1 {
			extra = fmt.Sprintf(" count=%d", occ.Count)
		}
		fmt.Fprintf(w, "    %s%s @ %s\n", name, extra, occ.Location)
		if occ.Mastering != nil {
			fmt.Fprintf(w, "      MDCV %s\n", printMastering(occ.Mastering))
		}
		if occ.Light != nil {
			fmt.Fprintf(w, "      CLLI MaxCLL=%d MaxFALL=%d\n",
				occ.Light.MaxCLL, occ.Light.MaxFALL)
		}
	}
}

func printMastering(m *MasteringView) string {
	var b strings.Builder
	for _, name := range []string{"G", "B", "R"} {
		p := m.Primaries[name]
		fmt.Fprintf(&b, "%s(%.5f,%.5f) ", name, p.X, p.Y)
	}
	fmt.Fprintf(&b, "WP(%.5f,%.5f) L[min=%.4f,max=%.4f] raw_luminance=[%d,%d]",
		m.WhitePoint.X, m.WhitePoint.Y,
		m.LuminanceNits.Min, m.LuminanceNits.Max,
		m.LuminanceNits.MinRaw, m.LuminanceNits.MaxRaw)
	return b.String()
}

func formatIssue(w io.Writer, issue hdr.Issue) {
	loc := ""
	if issue.Location != "" {
		loc = " @ " + issue.Location
	}
	src := ""
	if issue.Source != "" {
		src = "[" + string(issue.Source) + "]"
	}
	fmt.Fprintf(w, "    %s%s %s%s: %s\n",
		strings.ToUpper(string(issue.Severity)), src, issue.Code, loc, issue.Message)
}
