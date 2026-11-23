package scene

import (
	"encoding/json"
	"fmt"
)

type state struct {
	sceneID    string
	layouts    []SpeakerLayout
	programs   []Program
	objects    []Object
	extensions map[string]json.RawMessage
	issues     []UnresolvedField
}

func stateFromConfig(cfg *Config, envelope map[string]json.RawMessage) *state {
	if cfg == nil {
		return nil
	}
	s := &state{
		sceneID:    cfg.SceneID,
		layouts:    cloneLayouts(cfg.Layouts),
		programs:   clonePrograms(cfg.Programs),
		objects:    cloneObjects(cfg.Objects),
		extensions: joinExtensions(cfg.Extensions, envelope),
		issues:     append([]UnresolvedField(nil), cfg.Unresolved...),
	}
	return s
}

func (s *state) clone() *state {
	if s == nil {
		return nil
	}
	return &state{
		sceneID:    s.sceneID,
		layouts:    cloneLayouts(s.layouts),
		programs:   clonePrograms(s.programs),
		objects:    cloneObjects(s.objects),
		extensions: cloneExtensions(s.extensions),
		issues:     append([]UnresolvedField(nil), s.issues...),
	}
}

func (s *state) snapshot() *Snapshot {
	if s == nil {
		return nil
	}
	return &Snapshot{
		SceneID:  s.sceneID,
		Layouts:  cloneLayouts(s.layouts),
		Programs: clonePrograms(s.programs),
		Objects:  cloneObjects(s.objects),
		Ext:      cloneExtensions(s.extensions),
	}
}

func cloneLayouts(in []SpeakerLayout) []SpeakerLayout {
	if in == nil {
		return []SpeakerLayout{}
	}
	out := make([]SpeakerLayout, len(in))
	for i, layout := range in {
		out[i] = cloneLayout(layout)
	}
	return out
}

func cloneLayout(in SpeakerLayout) SpeakerLayout {
	out := in
	out.Channels = append([]Channel(nil), in.Channels...)
	if out.Channels == nil {
		out.Channels = []Channel{}
	}
	out.Extensions = cloneExtensions(in.Extensions)
	for i := range out.Channels {
		out.Channels[i].Extensions = cloneExtensions(in.Channels[i].Extensions)
	}
	return out
}

func clonePrograms(in []Program) []Program {
	if in == nil {
		return []Program{}
	}
	out := make([]Program, len(in))
	for i, program := range in {
		out[i] = program
		out[i].ObjectIDs = append([]string(nil), program.ObjectIDs...)
		if out[i].ObjectIDs == nil {
			out[i].ObjectIDs = []string{}
		}
		out[i].Extensions = cloneExtensions(program.Extensions)
	}
	return out
}

func cloneObjects(in []Object) []Object {
	if in == nil {
		return []Object{}
	}
	out := make([]Object, len(in))
	for i, object := range in {
		out[i] = object
		out[i].Channels = append([]string(nil), object.Channels...)
		out[i].Extensions = cloneExtensions(object.Extensions)
		if object.LayoutChannels != nil {
			out[i].LayoutChannels = make(map[string][]string, len(object.LayoutChannels))
			for layoutID, channels := range object.LayoutChannels {
				out[i].LayoutChannels[layoutID] = append([]string(nil), channels...)
			}
		}
	}
	return out
}

func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: configuration is missing", ErrInvalidSegment)
	}
	layoutIDs := make(map[string]bool, len(cfg.Layouts))
	for i, layout := range cfg.Layouts {
		if layout.ID == "" {
			return fmt.Errorf("%w: layouts[%d].id is empty", ErrInvalidSegment, i)
		}
		if layoutIDs[layout.ID] {
			return fmt.Errorf("%w: duplicate layout ID %q", ErrInvalidSegment, layout.ID)
		}
		layoutIDs[layout.ID] = true
		channelIDs := make(map[string]bool, len(layout.Channels))
		for j, channel := range layout.Channels {
			if channel.ID == "" {
				return fmt.Errorf("%w: layouts[%d].channels[%d].id is empty", ErrInvalidSegment, i, j)
			}
			if channelIDs[channel.ID] {
				return fmt.Errorf("%w: layout %q has duplicate channel %q", ErrInvalidSegment, layout.ID, channel.ID)
			}
			channelIDs[channel.ID] = true
		}
	}
	programIDs := make(map[string]bool, len(cfg.Programs))
	for i, program := range cfg.Programs {
		if program.ID == "" {
			return fmt.Errorf("%w: programs[%d].id is empty", ErrInvalidSegment, i)
		}
		if programIDs[program.ID] {
			return fmt.Errorf("%w: duplicate program ID %q", ErrInvalidSegment, program.ID)
		}
		programIDs[program.ID] = true
	}
	objectIDs := make(map[string]bool, len(cfg.Objects))
	for i, object := range cfg.Objects {
		if object.ID == "" {
			return fmt.Errorf("%w: objects[%d].id is empty", ErrInvalidSegment, i)
		}
		if objectIDs[object.ID] {
			return fmt.Errorf("%w: duplicate object ID %q", ErrInvalidSegment, object.ID)
		}
		objectIDs[object.ID] = true
	}
	return nil
}

func indexLayouts(layouts []SpeakerLayout) map[string]int {
	index := make(map[string]int, len(layouts))
	for i := range layouts {
		index[layouts[i].ID] = i
	}
	return index
}

func indexPrograms(programs []Program) map[string]int {
	index := make(map[string]int, len(programs))
	for i := range programs {
		index[programs[i].ID] = i
	}
	return index
}

func indexObjects(objects []Object) map[string]int {
	index := make(map[string]int, len(objects))
	for i := range objects {
		index[objects[i].ID] = i
	}
	return index
}

func appendMissingID(path, reason string, id string, issues *[]UnresolvedField) {
	*issues = append(*issues, unresolved(path, reason, id))
}
