package diff

import "ediff/internal/project"

func (e *Engine) diffTracks() {
	baseTracks := make(map[string]project.FlatTrack)
	targetTracks := make(map[string]project.FlatTrack)
	for _, ft := range e.base.Tracks {
		baseTracks[ft.Track.ID] = ft
	}
	for _, ft := range e.target.Tracks {
		targetTracks[ft.Track.ID] = ft
	}

	for id, bf := range baseTracks {
		tf, ok := targetTracks[id]
		if !ok {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "track.remove", ID: id, Track: pathString(bf.Path),
				Detail: "track present only in base",
			})
			continue
		}
		if fields := diffTrackFields(bf.Track, tf.Track); len(fields) > 0 {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "track.modify", ID: id,
				Track:  pathString(tf.Path),
				Fields: fields,
			})
		}
		if pathString(bf.Path) != pathString(tf.Path) {
			e.report.Conflicts = append(e.report.Conflicts, Conflict{
				Kind: "NESTED_TRACK", ID: id, Side: "both",
				Detail: "track moved nesting " + pathString(bf.Path) + " -> " + pathString(tf.Path),
			})
		}
	}

	for id, tf := range targetTracks {
		if _, ok := baseTracks[id]; !ok {
			e.report.Changes = append(e.report.Changes, Change{
				Op: "track.add", ID: id, Track: pathString(tf.Path),
				Detail: "track present only in target",
			})
		}
	}
}

func diffTrackFields(b, t project.Track) []FieldChange {
	var out []FieldChange
	if b.Name != t.Name {
		out = append(out, FieldChange{Path: "name", From: b.Name, To: t.Name})
	}
	if b.Kind != t.Kind {
		out = append(out, FieldChange{Path: "kind", From: b.Kind, To: t.Kind})
	}
	if boolVal(b.Muted) != boolVal(t.Muted) {
		out = append(out, FieldChange{Path: "muted", From: boolVal(b.Muted), To: boolVal(t.Muted)})
	}
	if boolVal(b.Locked) != boolVal(t.Locked) {
		out = append(out, FieldChange{Path: "locked", From: boolVal(b.Locked), To: boolVal(t.Locked)})
	}
	return out
}

func boolVal(p *bool) bool {
	return p != nil && *p
}
