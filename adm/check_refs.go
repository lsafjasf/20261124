package adm

import "fmt"

// idSets indexes every definable ADM element by ID for reference lookup.
type idSets struct {
	programmes map[string]bool
	contents   map[string]bool
	objects    map[string]bool
	packs      map[string]bool
	channels   map[string]bool
	streams    map[string]bool
	tracks     map[string]bool
	uids       map[string]bool
}

func indexDoc(d *Document) idSets {
	s := idSets{
		programmes: map[string]bool{},
		contents:   map[string]bool{},
		objects:    map[string]bool{},
		packs:      map[string]bool{},
		channels:   map[string]bool{},
		streams:    map[string]bool{},
		tracks:     map[string]bool{},
		uids:       map[string]bool{},
	}
	for _, p := range d.Programmes {
		s.programmes[p.ID] = true
	}
	for _, c := range d.Contents {
		s.contents[c.ID] = true
	}
	for _, o := range d.Objects {
		s.objects[o.ID] = true
	}
	for _, p := range d.Packs {
		s.packs[p.ID] = true
	}
	for _, c := range d.Channels {
		s.channels[c.ID] = true
	}
	for _, st := range d.Streams {
		s.streams[st.ID] = true
	}
	for _, t := range d.Tracks {
		s.tracks[t.ID] = true
	}
	for _, u := range d.TrackUIDs {
		s.uids[u.UID] = true
	}
	return s
}

// checkXMLRefs verifies that every intra-XML reference resolves to a
// defined element (missing-reference detection).
func checkXMLRefs(d *Document) []Finding {
	s := indexDoc(d)
	var out []Finding
	miss := func(from, refKind, id string) {
		out = append(out, Finding{Error, "MISSING-REF",
			fmt.Sprintf("%s references %s %s, which is not defined in axml", from, refKind, id)})
	}
	for _, p := range d.Programmes {
		for _, ref := range p.ContentRefIDs {
			if !s.contents[ref] {
				miss("audioProgramme "+p.ID, "audioContent", ref)
			}
		}
	}
	for _, c := range d.Contents {
		for _, ref := range c.ObjectRefIDs {
			if !s.objects[ref] {
				miss("audioContent "+c.ID, "audioObject", ref)
			}
		}
	}
	for _, o := range d.Objects {
		for _, ref := range o.PackRefIDs {
			if !s.packs[ref] {
				miss("audioObject "+o.ID, "audioPackFormat", ref)
			}
		}
		for _, ref := range o.TrackUIDRefs {
			if !s.uids[ref] {
				miss("audioObject "+o.ID, "audioTrackUID", ref)
			}
		}
		for _, ref := range o.ObjectRefIDs {
			if !s.objects[ref] {
				miss("audioObject "+o.ID, "audioObject", ref)
			}
		}
	}
	for _, p := range d.Packs {
		for _, ref := range p.ChannelRefIDs {
			if !s.channels[ref] {
				miss("audioPackFormat "+p.ID, "audioChannelFormat", ref)
			}
		}
		for _, ref := range p.PackRefIDs {
			if !s.packs[ref] {
				miss("audioPackFormat "+p.ID, "audioPackFormat", ref)
			}
		}
	}
	for _, st := range d.Streams {
		if st.ChannelRefID != "" && !s.channels[st.ChannelRefID] {
			miss("audioStreamFormat "+st.ID, "audioChannelFormat", st.ChannelRefID)
		}
		if st.PackRefID != "" && !s.packs[st.PackRefID] {
			miss("audioStreamFormat "+st.ID, "audioPackFormat", st.PackRefID)
		}
		if st.TrackRefID != "" && !s.tracks[st.TrackRefID] {
			miss("audioStreamFormat "+st.ID, "audioTrackFormat", st.TrackRefID)
		}
	}
	for _, t := range d.Tracks {
		if t.StreamRefID != "" && !s.streams[t.StreamRefID] {
			miss("audioTrackFormat "+t.ID, "audioStreamFormat", t.StreamRefID)
		}
	}
	for _, u := range d.TrackUIDs {
		if u.TrackRefID != "" && !s.tracks[u.TrackRefID] {
			miss("audioTrackUID "+u.UID, "audioTrackFormat", u.TrackRefID)
		}
		if u.PackRefID != "" && !s.packs[u.PackRefID] {
			miss("audioTrackUID "+u.UID, "audioPackFormat", u.PackRefID)
		}
	}
	return out
}

// checkChnaVsXML cross-checks the chna table against the XML in both
// directions: chna rows pointing at nothing, and XML UIDs never wired to a
// physical track.
func checkChnaVsXML(c *Chna, d *Document) []Finding {
	s := indexDoc(d)
	var out []Finding
	inChna := map[string]bool{}
	for _, e := range c.Entries {
		inChna[e.UID] = true
		if !s.uids[e.UID] {
			out = append(out, Finding{Error, "CHNA-ORPHAN",
				fmt.Sprintf("chna track %d uses UID %s, but axml defines no such audioTrackUID", e.TrackIndex, e.UID)})
		}
		if e.TrackRef != "" && !s.tracks[e.TrackRef] {
			out = append(out, Finding{Error, "CHNA-BAD-TRACKREF",
				fmt.Sprintf("chna track %d references audioTrackFormat %s, not defined in axml", e.TrackIndex, e.TrackRef)})
		}
		if e.PackRef != "" && !s.packs[e.PackRef] {
			out = append(out, Finding{Error, "CHNA-BAD-PACKREF",
				fmt.Sprintf("chna track %d references audioPackFormat %s, not defined in axml", e.TrackIndex, e.PackRef)})
		}
	}
	for _, u := range d.TrackUIDs {
		if !inChna[u.UID] {
			out = append(out, Finding{Error, "UID-NOT-IN-CHNA",
				fmt.Sprintf("axml defines audioTrackUID %s but no chna row attaches it to a physical track", u.UID)})
		}
	}
	if int(c.NumUIDs) != len(c.Entries) {
		out = append(out, Finding{Warn, "CHNA-COUNT",
			fmt.Sprintf("chna header declares %d UIDs but contains %d rows", c.NumUIDs, len(c.Entries))})
	}
	return out
}

// checkChnaVsFmt compares the chna table with the fmt/data chunks.
func checkChnaVsFmt(c *Chna, r *RIFF) []Finding {
	var out []Finding
	if !r.HasFmt {
		return out
	}
	if int(c.NumTracks) != int(r.Format.NumChannels) {
		out = append(out, Finding{Error, "TRACK-COUNT",
			fmt.Sprintf("chna declares %d tracks but fmt chunk reports %d channels", c.NumTracks, r.Format.NumChannels)})
	}
	for _, e := range c.Entries {
		if e.TrackIndex == 0 || e.TrackIndex > r.Format.NumChannels {
			out = append(out, Finding{Error, "TRACK-RANGE",
				fmt.Sprintf("chna row for UID %s points at track %d, outside 1..%d", e.UID, e.TrackIndex, r.Format.NumChannels)})
		}
	}
	return out
}

// checkAttachment finds objects that exist in the XML but are not attached
// to any physical audio track (declared but not deliverable as audio).
func checkAttachment(f *File) []Finding {
	var out []Finding
	inChna := map[string]uint16{}
	if f.Chna != nil {
		for _, e := range f.Chna.Entries {
			inChna[e.UID] = e.TrackIndex
		}
	}
	for _, o := range f.Doc.Objects {
		if len(o.TrackUIDRefs) == 0 {
			// Objects that only nest other objects need no direct wiring.
			if len(o.ObjectRefIDs) > 0 {
				continue
			}
			out = append(out, Finding{Error, "OBJ-NO-TRACK",
				fmt.Sprintf("audioObject %s (%q) has no audioTrackUIDRef: declared in XML but no audio attached", o.ID, o.Name)})
			continue
		}
		for _, ref := range o.TrackUIDRefs {
			if f.Chna == nil {
				break
			}
			if _, ok := inChna[ref]; !ok {
				out = append(out, Finding{Error, "OBJ-UNATTACHED",
					fmt.Sprintf("audioObject %s (%q) references audioTrackUID %s, which has no chna row: object declared but audio not attached", o.ID, o.Name, ref)})
			}
		}
	}
	return out
}
