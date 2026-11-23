package diff

import "ediff/internal/project"

func (e *Engine) diffMedia() {
	baseMedia := make(map[string]project.Media)
	targetMedia := make(map[string]project.Media)
	for _, m := range e.base.Project.Media {
		baseMedia[m.ID] = m
	}
	for _, m := range e.target.Project.Media {
		targetMedia[m.ID] = m
	}

	for id, bm := range baseMedia {
		tm, ok := targetMedia[id]
		if !ok {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "media.remove", ID: id,
				Detail: "media present only in base",
			})
			continue
		}
		if bm.Path != tm.Path || bm.Checksum != tm.Checksum {
			e.report.Conflicts = append(e.report.Conflicts, Conflict{
				Kind: "MEDIA_CONTENT_CONFLICT", ID: id, Side: "both",
				Detail: mediaConflictDetail(bm, tm),
			})
		}
	}

	for id := range targetMedia {
		if _, ok := baseMedia[id]; !ok {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "media.add", ID: id,
				Detail: "media present only in target",
			})
		}
	}
}

func mediaConflictDetail(b, t project.Media) string {
	out := ""
	if b.Path != t.Path {
		out += "path " + b.Path + " -> " + t.Path + "; "
	}
	if b.Checksum != t.Checksum {
		out += "checksum " + b.Checksum + " -> " + t.Checksum
	}
	return out
}
