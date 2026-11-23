package scene

import (
	"encoding/json"
	"fmt"
)

func validateEntityIDs(kind string, ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("%w: %s contains an empty ID", ErrInvalidSegment, kind)
		}
		if seen[id] {
			return fmt.Errorf("%w: %s contains duplicate ID %q", ErrInvalidSegment, kind, id)
		}
		seen[id] = true
	}
	return nil
}

func applyUpdate(st *state, update *Update) ([]UnresolvedField, error) {
	if update == nil {
		return nil, fmt.Errorf("%w: update payload is missing", ErrInvalidSegment)
	}
	if err := validateEntityIDs("deleteLayouts", update.DeleteLayouts); err != nil {
		return nil, err
	}
	if err := validateEntityIDs("deletePrograms", update.DeletePrograms); err != nil {
		return nil, err
	}
	if err := validateEntityIDs("deleteObjects", update.DeleteObjects); err != nil {
		return nil, err
	}
	layoutIDs := make(map[string]bool, len(update.Layouts))
	for i, layout := range update.Layouts {
		if layout.ID == "" {
			return nil, fmt.Errorf("%w: update layouts[%d].id is empty", ErrInvalidSegment, i)
		}
		if layoutIDs[layout.ID] {
			return nil, fmt.Errorf("%w: duplicate layout upsert %q", ErrInvalidSegment, layout.ID)
		}
		layoutIDs[layout.ID] = true
		channelIDs := make(map[string]bool, len(layout.Channels))
		for j, channel := range layout.Channels {
			if channel.ID == "" {
				return nil, fmt.Errorf("%w: update layouts[%d].channels[%d].id is empty", ErrInvalidSegment, i, j)
			}
			if channelIDs[channel.ID] {
				return nil, fmt.Errorf("%w: layout %q has duplicate channel %q", ErrInvalidSegment, layout.ID, channel.ID)
			}
			channelIDs[channel.ID] = true
		}
	}
	programIDs := make(map[string]bool, len(update.Programs))
	for i, patch := range update.Programs {
		if patch.ID == "" {
			return nil, fmt.Errorf("%w: update programs[%d].id is empty", ErrInvalidSegment, i)
		}
		if programIDs[patch.ID] {
			return nil, fmt.Errorf("%w: duplicate program patch %q", ErrInvalidSegment, patch.ID)
		}
		programIDs[patch.ID] = true
	}
	objectIDs := make(map[string]bool, len(update.Objects))
	for i, patch := range update.Objects {
		if patch.ID == "" {
			return nil, fmt.Errorf("%w: update objects[%d].id is empty", ErrInvalidSegment, i)
		}
		if objectIDs[patch.ID] {
			return nil, fmt.Errorf("%w: duplicate object patch %q", ErrInvalidSegment, patch.ID)
		}
		objectIDs[patch.ID] = true
	}

	deleteLayoutSet := stringSet(update.DeleteLayouts)
	deleteProgramSet := stringSet(update.DeletePrograms)
	deleteObjectSet := stringSet(update.DeleteObjects)
	var issues []UnresolvedField
	for id := range layoutIDs {
		if deleteLayoutSet[id] {
			issues = append(issues, unresolved("layouts["+id+"]", "conflicting_delete_and_upsert", id))
		}
	}
	for id := range programIDs {
		if deleteProgramSet[id] {
			issues = append(issues, unresolved("programs["+id+"]", "conflicting_delete_and_upsert", id))
		}
	}
	for id := range objectIDs {
		if deleteObjectSet[id] {
			issues = append(issues, unresolved("objects["+id+"]", "conflicting_delete_and_upsert", id))
		}
	}

	st.layouts, issues = deleteLayouts(st.layouts, deleteLayoutSet, issues)
	st.programs, issues = deletePrograms(st.programs, deleteProgramSet, issues)
	st.objects, issues = deleteObjects(st.objects, deleteObjectSet, issues)

	layoutIndex := indexLayouts(st.layouts)
	for _, layout := range update.Layouts {
		if deleteLayoutSet[layout.ID] {
			continue
		}
		replacement := cloneLayout(layout)
		if idx, ok := layoutIndex[layout.ID]; ok {
			st.layouts[idx] = replacement
		} else {
			layoutIndex[layout.ID] = len(st.layouts)
			st.layouts = append(st.layouts, replacement)
		}
	}

	programIndex := indexPrograms(st.programs)
	for _, patch := range update.Programs {
		if deleteProgramSet[patch.ID] {
			continue
		}
		idx, exists := programIndex[patch.ID]
		if !exists {
			idx = len(st.programs)
			programIndex[patch.ID] = idx
			st.programs = append(st.programs, Program{ID: patch.ID})
		}
		program := &st.programs[idx]
		if patch.LayoutID != nil {
			program.LayoutID = *patch.LayoutID
		}
		if patch.ObjectIDs != nil {
			program.ObjectIDs = append([]string(nil), (*patch.ObjectIDs)...)
		}
		program.Extensions = joinExtensions(program.Extensions, patch.Extensions)
	}

	objectIndex := indexObjects(st.objects)
	for _, patch := range update.Objects {
		if deleteObjectSet[patch.ID] {
			continue
		}
		idx, exists := objectIndex[patch.ID]
		if !exists {
			idx = len(st.objects)
			objectIndex[patch.ID] = idx
			st.objects = append(st.objects, Object{ID: patch.ID})
		}
		object := &st.objects[idx]
		if patch.Channels != nil {
			object.Channels = append([]string(nil), (*patch.Channels)...)
		}
		if patch.LayoutChannels != nil {
			if *patch.LayoutChannels == nil {
				object.LayoutChannels = nil
			} else {
				object.LayoutChannels = cloneBindings(*patch.LayoutChannels)
			}
		}
		object.Extensions = joinExtensions(object.Extensions, patch.Extensions)
	}
	st.extensions = joinExtensions(st.extensions, update.Extensions)
	return issues, nil
}

func cloneBindings(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func deleteLayouts(in []SpeakerLayout, remove map[string]bool, issues []UnresolvedField) ([]SpeakerLayout, []UnresolvedField) {
	out := in[:0]
	found := make(map[string]bool, len(remove))
	for _, layout := range in {
		if remove[layout.ID] {
			found[layout.ID] = true
			continue
		}
		out = append(out, layout)
	}
	for id := range remove {
		if !found[id] {
			issues = append(issues, unresolved("deleteLayouts["+id+"]", "unknown_layout_to_delete", id))
		}
	}
	return out, issues
}

func deletePrograms(in []Program, remove map[string]bool, issues []UnresolvedField) ([]Program, []UnresolvedField) {
	out := in[:0]
	found := make(map[string]bool, len(remove))
	for _, program := range in {
		if remove[program.ID] {
			found[program.ID] = true
			continue
		}
		out = append(out, program)
	}
	for id := range remove {
		if !found[id] {
			issues = append(issues, unresolved("deletePrograms["+id+"]", "unknown_program_to_delete", id))
		}
	}
	return out, issues
}

func deleteObjects(in []Object, remove map[string]bool, issues []UnresolvedField) ([]Object, []UnresolvedField) {
	out := in[:0]
	found := make(map[string]bool, len(remove))
	for _, object := range in {
		if remove[object.ID] {
			found[object.ID] = true
			continue
		}
		out = append(out, object)
	}
	for id := range remove {
		if !found[id] {
			issues = append(issues, unresolved("deleteObjects["+id+"]", "unknown_object_to_delete", id))
		}
	}
	return out, issues
}

func mergeStateExtensions(st *state, envelope map[string]json.RawMessage) {
	if len(envelope) == 0 {
		return
	}
	st.extensions = joinExtensions(st.extensions, envelope)
}
