package diff

import (
	"sort"

	"ediff/internal/project"
)

// Engine 在两份已加载工程之间计算最小变更单。
type Engine struct {
	base   *project.Loaded
	target *project.Loaded
	report *Report
}

// Compare 计算 base -> target 的变更单。
func Compare(base, target *project.Loaded) *Report {
	e := &Engine{
		base:   base,
		target: target,
		report: &Report{
			Inputs: InputMeta{
				BasePath: base.Source, BaseSHA256: base.SHA256,
				TargetPath: target.Source, TargetSHA256: target.SHA256,
			},
			Changes:   []Change{},
			Conflicts: []Conflict{},
		},
	}
	e.collectIssues()
	e.diffMedia()
	e.diffTracks()
	e.diffClips()
	e.detectNameMediaConflicts()
	e.finalize()
	return e.report
}

func (e *Engine) collectIssues() {
	collect := func(side string, l *project.Loaded) {
		for _, is := range l.Issues {
			e.report.Conflicts = append(e.report.Conflicts, Conflict{
				Kind: mapIssueKind(is.Kind),
				ID:   is.ID, Track: is.Track, Side: side, Detail: is.Detail,
			})
		}
	}
	collect("base", e.base)
	collect("target", e.target)
}

func mapIssueKind(kind string) string {
	switch kind {
	case "CLIP_DUPLICATE_ID", "MEDIA_DUPLICATE_ID":
		if kind == "MEDIA_DUPLICATE_ID" {
			return "DUP_MEDIA_ID"
		}
		return "DUP_CLIP_ID"
	case "TRACK_DUPLICATE_ID":
		return "DUP_TRACK_ID"
	case "CLIP_UNKNOWN_MEDIA":
		return "CLIP_UNKNOWN_MEDIA"
	case "CLIP_BAD_SOURCE_RANGE":
		return "BAD_SOURCE_RANGE"
	default:
		return kind
	}
}

func (e *Engine) finalize() {
	sort.SliceStable(e.report.Changes, func(i, j int) bool {
		if e.report.Changes[i].Op != e.report.Changes[j].Op {
			return e.report.Changes[i].Op < e.report.Changes[j].Op
		}
		return e.report.Changes[i].ID < e.report.Changes[j].ID
	})
	sort.SliceStable(e.report.Conflicts, func(i, j int) bool {
		if e.report.Conflicts[i].Kind != e.report.Conflicts[j].Kind {
			return e.report.Conflicts[i].Kind < e.report.Conflicts[j].Kind
		}
		if e.report.Conflicts[i].ID != e.report.Conflicts[j].ID {
			return e.report.Conflicts[i].ID < e.report.Conflicts[j].ID
		}
		return e.report.Conflicts[i].Side < e.report.Conflicts[j].Side
	})

	for _, c := range e.report.Changes {
		switch c.Op {
		case "track.add":
			e.report.Summary.TracksAdded++
		case "track.remove":
			e.report.Summary.TracksRemoved++
		case "track.modify":
			e.report.Summary.TracksModified++
		case "clip.add":
			e.report.Summary.ClipsAdded++
		case "clip.remove":
			e.report.Summary.ClipsRemoved++
		case "clip.modify":
			e.report.Summary.ClipsModified++
		case "clip.move":
			e.report.Summary.ClipsMoved++
		case "media.add":
			e.report.Summary.MediaAdded++
		case "media.remove":
			e.report.Summary.MediaRemoved++
		}
	}
	e.report.Summary.Conflicts = len(e.report.Conflicts)
}

func pathString(p []string) string {
	out := ""
	for i, s := range p {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}
