package report

import (
	"fmt"
	"io"
	"strings"

	"ediff/internal/diff"
)

// WriteText 输出便于人工核查的纯文本变更单。
func WriteText(w io.Writer, r *diff.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "变更单 (ediff)\n")
	fmt.Fprintf(&b, "base   : %s  sha256=%s\n", r.Inputs.BasePath, r.Inputs.BaseSHA256)
	fmt.Fprintf(&b, "target : %s  sha256=%s\n\n", r.Inputs.TargetPath, r.Inputs.TargetSHA256)

	s := r.Summary
	fmt.Fprintf(&b, "摘要: 轨道 +%d/-%d/~%d  片段 +%d/-%d/~%d/move %d  素材 +%d/-%d  冲突 %d\n\n",
		s.TracksAdded, s.TracksRemoved, s.TracksModified,
		s.ClipsAdded, s.ClipsRemoved, s.ClipsModified, s.ClipsMoved,
		s.MediaAdded, s.MediaRemoved, s.Conflicts)

	if len(r.Changes) > 0 {
		b.WriteString("最小修改列表:\n")
		for _, c := range r.Changes {
			fmt.Fprintf(&b, "  - %-13s %s", c.Op, c.ID)
			if c.Track != "" {
				fmt.Fprintf(&b, "  track=%s", c.Track)
			}
			if c.FromTrack != "" || c.ToTrack != "" {
				fmt.Fprintf(&b, "  %s -> %s", c.FromTrack, c.ToTrack)
			}
			b.WriteString("\n")
			for _, f := range c.Fields {
				fmt.Fprintf(&b, "      %s: %v -> %v\n", f.Path, f.From, f.To)
			}
			if c.Detail != "" {
				fmt.Fprintf(&b, "      note: %s\n", c.Detail)
			}
		}
		b.WriteString("\n")
	}

	if len(r.Conflicts) > 0 {
		b.WriteString("冲突项 (合并前必须人工裁决):\n")
		for _, c := range r.Conflicts {
			fmt.Fprintf(&b, "  ! %-22s [%s] %s  %s\n", c.Kind, c.Side, c.ID, c.Detail)
			if c.Track != "" {
				fmt.Fprintf(&b, "      track=%s\n", c.Track)
			}
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}
