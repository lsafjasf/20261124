package scene

import (
	"fmt"
	"sort"
)

func resolveScene(st *state) ([]ObjectChannelMapping, []UnresolvedField) {
	if st == nil {
		return []ObjectChannelMapping{}, nil
	}
	issues := append([]UnresolvedField(nil), st.issues...)
	issues = append(issues, collectExtensionIssues(st)...)
	mappings := make([]ObjectChannelMapping, 0)

	layoutIndex := indexLayouts(st.layouts)
	objectIndex := indexObjects(st.objects)

	for _, object := range st.objects {
		layoutIDs := make([]string, 0, len(object.LayoutChannels))
		for layoutID := range object.LayoutChannels {
			layoutIDs = append(layoutIDs, layoutID)
		}
		sort.Strings(layoutIDs)
		for _, layoutID := range layoutIDs {
			if _, ok := layoutIndex[layoutID]; !ok {
				issues = append(issues, unresolved(
					fmt.Sprintf("objects[%q].layoutChannels[%q]", object.ID, layoutID),
					"unknown_layout_binding", object.LayoutChannels[layoutID]))
			}
		}
	}

	for _, program := range st.programs {
		layout, hasLayout := resolveProgramLayout(program, layoutIndex, st.layouts, &issues)
		var channelIndex map[string]int
		if hasLayout {
			channelIndex = make(map[string]int, len(layout.Channels))
			for i, channel := range layout.Channels {
				channelIndex[channel.ID] = i
			}
		}
		seenInProgram := make(map[string]bool, len(program.ObjectIDs))
		for _, objectID := range program.ObjectIDs {
			if seenInProgram[objectID] {
				issues = append(issues, unresolved(
					fmt.Sprintf("programs[%q].objectIds", program.ID),
					"duplicate_object_reference", objectID))
				continue
			}
			seenInProgram[objectID] = true
			idx, ok := objectIndex[objectID]
			if !ok {
				issues = append(issues, unresolved(
					fmt.Sprintf("programs[%q].objectIds", program.ID),
					"missing_object", objectID))
				continue
			}
			object := st.objects[idx]
			if !hasLayout {
				continue
			}
			binding, ok, reason := bindingsForLayout(object, program.LayoutID)
			if !ok {
				path := binding.path
				if path == "" {
					path = fmt.Sprintf("objects[%q].layoutChannels[%q]", object.ID, program.LayoutID)
				}
				issues = append(issues, unresolved(
					path, reason, program.LayoutID))
				continue
			}
			channels, channelIssues := resolveChannels(
				binding, channelIndex, layout.Channels)
			issues = append(issues, channelIssues...)
			if len(channels) == 0 {
				continue
			}
			mappings = append(mappings, ObjectChannelMapping{
				ObjectID:  object.ID,
				ProgramID: program.ID,
				LayoutID:  program.LayoutID,
				Channels:  channels,
			})
		}
	}
	return mappings, normalizeIssues(issues)
}

func resolveProgramLayout(program Program, index map[string]int, layouts []SpeakerLayout, issues *[]UnresolvedField) (SpeakerLayout, bool) {
	if program.LayoutID == "" {
		*issues = append(*issues, unresolved(
			fmt.Sprintf("programs[%q].layoutId", program.ID), "missing_layout_reference", nil))
		return SpeakerLayout{}, false
	}
	idx, ok := index[program.LayoutID]
	if !ok {
		*issues = append(*issues, unresolved(
			fmt.Sprintf("programs[%q].layoutId", program.ID), "unknown_layout", program.LayoutID))
		return SpeakerLayout{}, false
	}
	return layouts[idx], true
}

type channelBinding struct {
	values []string
	path   string
}

func bindingsForLayout(object Object, layoutID string) (channelBinding, bool, string) {
	if object.LayoutChannels != nil {
		if bindings, ok := object.LayoutChannels[layoutID]; ok {
			path := fmt.Sprintf("objects[%q].layoutChannels[%q]", object.ID, layoutID)
			if len(bindings) == 0 {
				return channelBinding{path: path}, false, "empty_channel_binding"
			}
			return channelBinding{
				values: bindings,
				path:   path,
			}, true, ""
		}
	}
	if object.Channels != nil {
		path := fmt.Sprintf("objects[%q].channels", object.ID)
		if len(object.Channels) == 0 {
			return channelBinding{path: path}, false, "empty_channel_binding"
		}
		return channelBinding{
			values: object.Channels,
			path:   path,
		}, true, ""
	}
	if len(object.LayoutChannels) > 0 {
		return channelBinding{}, false, "layout_mapping_missing"
	}
	return channelBinding{}, false, "missing_channel_binding"
}

func resolveChannels(binding channelBinding, channelIndex map[string]int, layoutChannels []Channel) ([]OutputChannel, []UnresolvedField) {
	var issues []UnresolvedField
	seen := make(map[string]bool, len(binding.values))
	channels := make([]OutputChannel, 0, len(binding.values))
	for i, channelID := range binding.values {
		path := fmt.Sprintf("%s[%d]", binding.path, i)
		if seen[channelID] {
			issues = append(issues, unresolved(path, "duplicate_channel_binding", channelID))
			continue
		}
		seen[channelID] = true
		index, ok := channelIndex[channelID]
		if !ok {
			issues = append(issues, unresolved(path, "unknown_output_channel", channelID))
			continue
		}
		channels = append(channels, OutputChannel{ID: layoutChannels[index].ID, Index: index})
	}
	return channels, issues
}

func collectExtensionIssues(st *state) []UnresolvedField {
	var issues []UnresolvedField
	issues = append(issues, sortedExtensionFields("extensions.", st.extensions)...)
	for _, layout := range st.layouts {
		prefix := fmt.Sprintf("layouts[%q].", layout.ID)
		issues = append(issues, sortedExtensionFields(prefix+"extensions.", layout.Extensions)...)
		for _, channel := range layout.Channels {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("layouts[%q].channels[%q].extensions.", layout.ID, channel.ID),
				channel.Extensions)...)
		}
	}
	for _, program := range st.programs {
		issues = append(issues, sortedExtensionFields(
			fmt.Sprintf("programs[%q].extensions.", program.ID), program.Extensions)...)
	}
	for _, object := range st.objects {
		issues = append(issues, sortedExtensionFields(
			fmt.Sprintf("objects[%q].extensions.", object.ID), object.Extensions)...)
	}
	return issues
}

func segmentExtensionIssues(segment Segment) []UnresolvedField {
	var issues []UnresolvedField
	issues = append(issues, sortedExtensionFields("extensions.", segment.Extensions)...)
	if segment.Config != nil {
		issues = append(issues, sortedExtensionFields("extensions.", segment.Config.Extensions)...)
		for _, layout := range segment.Config.Layouts {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("layouts[%q].extensions.", layout.ID), layout.Extensions)...)
			for _, channel := range layout.Channels {
				issues = append(issues, sortedExtensionFields(
					fmt.Sprintf("layouts[%q].channels[%q].extensions.", layout.ID, channel.ID),
					channel.Extensions)...)
			}
		}
		for _, program := range segment.Config.Programs {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("programs[%q].extensions.", program.ID), program.Extensions)...)
		}
		for _, object := range segment.Config.Objects {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("objects[%q].extensions.", object.ID), object.Extensions)...)
		}
	}
	if segment.Update != nil {
		issues = append(issues, sortedExtensionFields("extensions.", segment.Update.Extensions)...)
		for _, layout := range segment.Update.Layouts {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("layouts[%q].extensions.", layout.ID), layout.Extensions)...)
			for _, channel := range layout.Channels {
				issues = append(issues, sortedExtensionFields(
					fmt.Sprintf("layouts[%q].channels[%q].extensions.", layout.ID, channel.ID),
					channel.Extensions)...)
			}
		}
		for _, program := range segment.Update.Programs {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("programs[%q].extensions.", program.ID), program.Extensions)...)
		}
		for _, object := range segment.Update.Objects {
			issues = append(issues, sortedExtensionFields(
				fmt.Sprintf("objects[%q].extensions.", object.ID), object.Extensions)...)
		}
	}
	return issues
}

func normalizeIssues(in []UnresolvedField) []UnresolvedField {
	seen := make(map[string]bool, len(in))
	out := make([]UnresolvedField, 0, len(in))
	for _, issue := range in {
		key := issue.Path + "\x00" + issue.Reason + "\x00" + issue.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, issue)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Reason != out[j].Reason {
			return out[i].Reason < out[j].Reason
		}
		return out[i].Value < out[j].Value
	})
	return out
}
