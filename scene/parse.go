package scene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ErrInvalidSegment is returned when a segment is malformed or its required
// envelope fields are inconsistent.
var ErrInvalidSegment = errors.New("invalid scene segment")

type objectFields map[string]json.RawMessage

func decodeObjectFields(raw []byte, where string) (objectFields, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, fmt.Errorf("%w: %s is missing", ErrInvalidSegment, where)
	}
	if err := rejectDuplicateFields(raw); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidSegment, where, err)
	}
	var fields objectFields
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&fields); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidSegment, where, err)
	}
	if err := ensureEOF(dec); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidSegment, where, err)
	}
	return fields, nil
}

func rejectDuplicateFields(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return scanJSONValue(dec)
}

func scanJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = true
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}

func take(fields objectFields, key string) json.RawMessage {
	raw := fields[key]
	delete(fields, key)
	return raw
}

func unknownFields(fields objectFields, prefix string) []UnresolvedField {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]UnresolvedField, 0, len(keys))
	for _, key := range keys {
		out = append(out, UnresolvedField{
			Path:   prefix + key,
			Reason: "unknown_field",
			Value:  compactJSON(fields[key]),
		})
	}
	return out
}

func parseString(raw json.RawMessage, where string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%w: %s must be a string: %v", ErrInvalidSegment, where, err)
	}
	return value, nil
}

func parseStringSlice(raw json.RawMessage, where string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("%w: %s must be an array of strings: %v", ErrInvalidSegment, where, err)
	}
	return values, nil
}

func parseExtensions(raw json.RawMessage, where string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var ext map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ext); err != nil {
		return nil, fmt.Errorf("%w: %s must be an object: %v", ErrInvalidSegment, where, err)
	}
	if len(ext) == 0 {
		return nil, nil
	}
	return ext, nil
}

func parseChannel(raw json.RawMessage, where string) (Channel, []UnresolvedField, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return Channel{}, nil, err
	}
	var channel Channel
	idRaw := take(fields, "id")
	channel.ID, err = parseString(idRaw, where+".id")
	if err != nil {
		return Channel{}, nil, err
	}
	channel.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return Channel{}, nil, err
	}
	return channel, unknownFields(fields, where+"."), nil
}

func parseLayout(raw json.RawMessage, where string) (SpeakerLayout, []UnresolvedField, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return SpeakerLayout{}, nil, err
	}
	var layout SpeakerLayout
	layout.ID, err = parseString(take(fields, "id"), where+".id")
	if err != nil {
		return layout, nil, err
	}
	layout.Name, err = parseString(take(fields, "name"), where+".name")
	if err != nil {
		return layout, nil, err
	}
	layout.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return layout, nil, err
	}
	channelsRaw := take(fields, "channels")
	issues := unknownFields(fields, where+".")
	if len(channelsRaw) > 0 && !bytes.Equal(bytes.TrimSpace(channelsRaw), []byte("null")) {
		var items []json.RawMessage
		if err := json.Unmarshal(channelsRaw, &items); err != nil {
			return layout, issues, fmt.Errorf("%w: %s.channels must be an array: %v", ErrInvalidSegment, where, err)
		}
		layout.Channels = make([]Channel, 0, len(items))
		for i, item := range items {
			channel, nested, err := parseChannel(item, fmt.Sprintf("%s.channels[%d]", where, i))
			if err != nil {
				return layout, issues, err
			}
			issues = append(issues, nested...)
			layout.Channels = append(layout.Channels, channel)
		}
	}
	return layout, issues, nil
}

func parseProgram(raw json.RawMessage, where string) (Program, []UnresolvedField, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return Program{}, nil, err
	}
	var program Program
	program.ID, err = parseString(take(fields, "id"), where+".id")
	if err != nil {
		return program, nil, err
	}
	program.LayoutID, err = parseString(take(fields, "layoutId"), where+".layoutId")
	if err != nil {
		return program, nil, err
	}
	program.ObjectIDs, err = parseStringSlice(take(fields, "objectIds"), where+".objectIds")
	if err != nil {
		return program, nil, err
	}
	program.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return program, nil, err
	}
	return program, unknownFields(fields, where+"."), nil
}

func parseObject(raw json.RawMessage, where string) (Object, []UnresolvedField, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return Object{}, nil, err
	}
	var object Object
	object.ID, err = parseString(take(fields, "id"), where+".id")
	if err != nil {
		return object, nil, err
	}
	object.Channels, err = parseStringSlice(take(fields, "channels"), where+".channels")
	if err != nil {
		return object, nil, err
	}
	layoutChannelsRaw := take(fields, "layoutChannels")
	object.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return object, nil, err
	}
	issues := unknownFields(fields, where+".")
	if len(layoutChannelsRaw) > 0 && !bytes.Equal(bytes.TrimSpace(layoutChannelsRaw), []byte("null")) {
		var bindings map[string][]string
		if err := json.Unmarshal(layoutChannelsRaw, &bindings); err != nil {
			return object, issues, fmt.Errorf("%w: %s.layoutChannels must map layout IDs to string arrays: %v", ErrInvalidSegment, where, err)
		}
		object.LayoutChannels = bindings
	}
	return object, issues, nil
}

func parseRawArray(raw json.RawMessage, where string) ([]json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%w: %s must be an array: %v", ErrInvalidSegment, where, err)
	}
	return items, nil
}

func parseConfigObject(raw []byte, where string) (*Config, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	cfg.SceneID, err = parseString(take(fields, "sceneId"), where+".sceneId")
	if err != nil {
		return nil, err
	}
	cfg.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return nil, err
	}
	layoutItems, err := parseRawArray(take(fields, "layouts"), where+".layouts")
	if err != nil {
		return nil, err
	}
	programItems, err := parseRawArray(take(fields, "programs"), where+".programs")
	if err != nil {
		return nil, err
	}
	objectItems, err := parseRawArray(take(fields, "objects"), where+".objects")
	if err != nil {
		return nil, err
	}
	cfg.Unresolved = append(cfg.Unresolved, unknownFields(fields, where+".")...)
	for i, item := range layoutItems {
		layout, issues, err := parseLayout(item, fmt.Sprintf("%s.layouts[%d]", where, i))
		if err != nil {
			return nil, err
		}
		cfg.Unresolved = append(cfg.Unresolved, issues...)
		cfg.Layouts = append(cfg.Layouts, layout)
	}
	for i, item := range programItems {
		program, issues, err := parseProgram(item, fmt.Sprintf("%s.programs[%d]", where, i))
		if err != nil {
			return nil, err
		}
		cfg.Unresolved = append(cfg.Unresolved, issues...)
		cfg.Programs = append(cfg.Programs, program)
	}
	for i, item := range objectItems {
		object, issues, err := parseObject(item, fmt.Sprintf("%s.objects[%d]", where, i))
		if err != nil {
			return nil, err
		}
		cfg.Unresolved = append(cfg.Unresolved, issues...)
		cfg.Objects = append(cfg.Objects, object)
	}
	return cfg, nil
}

func parseProgramPatch(raw json.RawMessage, where string) (ProgramPatch, []UnresolvedField, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return ProgramPatch{}, nil, err
	}
	var patch ProgramPatch
	patch.ID, err = parseString(take(fields, "id"), where+".id")
	if err != nil {
		return patch, nil, err
	}
	layoutRaw := take(fields, "layoutId")
	if len(layoutRaw) > 0 {
		layoutID := ""
		if !bytes.Equal(bytes.TrimSpace(layoutRaw), []byte("null")) {
			layoutID, err = parseString(layoutRaw, where+".layoutId")
			if err != nil {
				return patch, nil, err
			}
		}
		patch.LayoutID = &layoutID
	}
	objectIDsRaw := take(fields, "objectIds")
	if len(objectIDsRaw) > 0 {
		objectIDs, err := parseStringSlice(objectIDsRaw, where+".objectIds")
		if err != nil {
			return patch, nil, err
		}
		patch.ObjectIDs = &objectIDs
	}
	patch.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return patch, nil, err
	}
	return patch, unknownFields(fields, where+"."), nil
}

func parseObjectPatch(raw json.RawMessage, where string) (ObjectPatch, []UnresolvedField, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return ObjectPatch{}, nil, err
	}
	var patch ObjectPatch
	patch.ID, err = parseString(take(fields, "id"), where+".id")
	if err != nil {
		return patch, nil, err
	}
	channelsRaw := take(fields, "channels")
	if len(channelsRaw) > 0 {
		channels, err := parseStringSlice(channelsRaw, where+".channels")
		if err != nil {
			return patch, nil, err
		}
		patch.Channels = &channels
	}
	layoutChannelsRaw := take(fields, "layoutChannels")
	if len(layoutChannelsRaw) > 0 {
		var bindings map[string][]string
		if !bytes.Equal(bytes.TrimSpace(layoutChannelsRaw), []byte("null")) {
			if err := json.Unmarshal(layoutChannelsRaw, &bindings); err != nil {
				return patch, nil, fmt.Errorf("%w: %s.layoutChannels must map layout IDs to string arrays: %v", ErrInvalidSegment, where, err)
			}
		}
		patch.LayoutChannels = &bindings
	}
	patch.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return patch, nil, err
	}
	return patch, unknownFields(fields, where+"."), nil
}

func parseUpdateObject(raw []byte, where string) (*Update, error) {
	fields, err := decodeObjectFields(raw, where)
	if err != nil {
		return nil, err
	}
	update := &Update{}
	update.Extensions, err = parseExtensions(take(fields, "extensions"), where+".extensions")
	if err != nil {
		return nil, err
	}
	update.DeleteLayouts, err = parseStringSlice(take(fields, "deleteLayouts"), where+".deleteLayouts")
	if err != nil {
		return nil, err
	}
	update.DeletePrograms, err = parseStringSlice(take(fields, "deletePrograms"), where+".deletePrograms")
	if err != nil {
		return nil, err
	}
	update.DeleteObjects, err = parseStringSlice(take(fields, "deleteObjects"), where+".deleteObjects")
	if err != nil {
		return nil, err
	}
	layoutItems, err := parseRawArray(take(fields, "layouts"), where+".layouts")
	if err != nil {
		return nil, err
	}
	programItems, err := parseRawArray(take(fields, "programs"), where+".programs")
	if err != nil {
		return nil, err
	}
	objectItems, err := parseRawArray(take(fields, "objects"), where+".objects")
	if err != nil {
		return nil, err
	}
	update.Unresolved = append(update.Unresolved, unknownFields(fields, where+".")...)
	for i, item := range layoutItems {
		layout, issues, err := parseLayout(item, fmt.Sprintf("%s.layouts[%d]", where, i))
		if err != nil {
			return nil, err
		}
		update.Unresolved = append(update.Unresolved, issues...)
		update.Layouts = append(update.Layouts, layout)
	}
	for i, item := range programItems {
		patch, issues, err := parseProgramPatch(item, fmt.Sprintf("%s.programs[%d]", where, i))
		if err != nil {
			return nil, err
		}
		update.Unresolved = append(update.Unresolved, issues...)
		update.Programs = append(update.Programs, patch)
	}
	for i, item := range objectItems {
		patch, issues, err := parseObjectPatch(item, fmt.Sprintf("%s.objects[%d]", where, i))
		if err != nil {
			return nil, err
		}
		update.Unresolved = append(update.Unresolved, issues...)
		update.Objects = append(update.Objects, patch)
	}
	return update, nil
}

func parseSequence(raw json.RawMessage, where string) (uint64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, fmt.Errorf("%w: %s must be an unsigned integer: %v", ErrInvalidSegment, where, err)
	}
	value, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s must be an unsigned integer: %v", ErrInvalidSegment, where, err)
	}
	return value, nil
}

func normalizeKind(raw json.RawMessage) (SegmentKind, error) {
	value, err := parseString(raw, "type")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "config", "configuration":
		return SegmentConfig, nil
	case "update", "delta":
		return SegmentUpdate, nil
	default:
		return "", fmt.Errorf("%w: unsupported segment type %q", ErrInvalidSegment, value)
	}
}

func parseSegment(raw []byte, hint *SegmentKind) (*Segment, error) {
	fields, err := decodeObjectFields(raw, "segment")
	if err != nil {
		return nil, err
	}
	var kindRaw json.RawMessage
	typeRaw := take(fields, "type")
	aliasKindRaw := take(fields, "kind")
	if len(typeRaw) > 0 && len(aliasKindRaw) > 0 {
		return nil, fmt.Errorf("%w: both type and kind are present", ErrInvalidSegment)
	}
	if len(typeRaw) > 0 {
		kindRaw = typeRaw
	} else {
		kindRaw = aliasKindRaw
	}
	kind, err := normalizeKind(kindRaw)
	if err != nil {
		return nil, err
	}

	sequenceRaw := take(fields, "sequence")
	effectiveRaw := take(fields, "effectiveSequence")
	effectiveSnakeRaw := take(fields, "effective_sequence")
	present := 0
	for _, item := range []json.RawMessage{sequenceRaw, effectiveRaw, effectiveSnakeRaw} {
		if len(item) > 0 {
			present++
			sequenceRaw = item
		}
	}
	if present > 1 {
		return nil, fmt.Errorf("%w: multiple sequence fields are present", ErrInvalidSegment)
	}
	sequence, err := parseSequence(sequenceRaw, "sequence")
	if err != nil {
		return nil, err
	}

	configRaw := take(fields, "config")
	updateRaw := take(fields, "update")
	if len(configRaw) > 0 && len(updateRaw) > 0 {
		return nil, fmt.Errorf("%w: both config and update payloads are present", ErrInvalidSegment)
	}
	extensionsRaw := take(fields, "extensions")

	if kind == "" {
		switch {
		case hint != nil:
			kind = *hint
		case len(configRaw) > 0:
			kind = SegmentConfig
		case len(updateRaw) > 0:
			kind = SegmentUpdate
		case hasAnyKey(fields, "deleteLayouts", "deletePrograms", "deleteObjects"):
			kind = SegmentUpdate
		case hasAnyKey(fields, "layouts", "programs", "objects", "sceneId"):
			kind = SegmentConfig
		default:
			return nil, fmt.Errorf("%w: segment type is missing", ErrInvalidSegment)
		}
	}
	if hint != nil && kind != *hint {
		return nil, fmt.Errorf("%w: expected %s segment, got %s", ErrInvalidSegment, *hint, kind)
	}

	segment := &Segment{Kind: kind, Sequence: sequence, HasSequence: len(sequenceRaw) > 0}
	switch kind {
	case SegmentConfig:
		if len(updateRaw) > 0 {
			return nil, fmt.Errorf("%w: update payload in config segment", ErrInvalidSegment)
		}
		if len(configRaw) > 0 {
			segment.Config, err = parseConfigObject(configRaw, "config")
			if err == nil {
				var ext map[string]json.RawMessage
				ext, err = parseExtensions(extensionsRaw, "segment.extensions")
				segment.Extensions = joinExtensions(segment.Extensions, ext)
			}
			segment.Unresolved = append(segment.Unresolved, unknownFields(fields, "segment.")...)
		} else {
			fields["extensions"] = extensionsRaw
			segment.Config, err = parseConfigObject(marshalFields(fields), "config")
		}
		if err == nil {
			segment.Unresolved = append(segment.Unresolved, segment.Config.Unresolved...)
		}
	case SegmentUpdate:
		if len(configRaw) > 0 {
			return nil, fmt.Errorf("%w: config payload in update segment", ErrInvalidSegment)
		}
		if len(updateRaw) > 0 {
			segment.Update, err = parseUpdateObject(updateRaw, "update")
			if err == nil {
				var ext map[string]json.RawMessage
				ext, err = parseExtensions(extensionsRaw, "segment.extensions")
				segment.Extensions = joinExtensions(segment.Extensions, ext)
			}
			segment.Unresolved = append(segment.Unresolved, unknownFields(fields, "segment.")...)
		} else {
			fields["extensions"] = extensionsRaw
			segment.Update, err = parseUpdateObject(marshalFields(fields), "update")
		}
		if err == nil {
			segment.Unresolved = append(segment.Unresolved, segment.Update.Unresolved...)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported segment type %q", ErrInvalidSegment, kind)
	}
	if err != nil {
		return nil, err
	}
	return segment, nil
}

func hasAnyKey(fields objectFields, keys ...string) bool {
	for _, key := range keys {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func marshalFields(fields objectFields) []byte {
	data, _ := json.Marshal(fields)
	return data
}

func joinExtensions(a, b map[string]json.RawMessage) map[string]json.RawMessage {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := cloneExtensions(a)
	if out == nil {
		out = make(map[string]json.RawMessage, len(b))
	}
	for key, value := range b {
		out[key] = value
	}
	return out
}

// ParseSegment parses one configuration or update segment.
func ParseSegment(raw []byte) (Segment, error) {
	segment, err := parseSegment(raw, nil)
	if err != nil {
		return Segment{}, err
	}
	return *segment, nil
}

// ParseConfig parses a configuration snapshot. Both a bare config object and
// a config segment envelope are accepted.
func ParseConfig(raw []byte) (*Config, error) {
	hint := SegmentConfig
	segment, err := parseSegment(raw, &hint)
	if err != nil {
		return nil, err
	}
	return segment.Config, nil
}

// ParseUpdate parses an update segment. Both a bare update object and an
// update segment envelope are accepted.
func ParseUpdate(raw []byte) (*Update, error) {
	hint := SegmentUpdate
	segment, err := parseSegment(raw, &hint)
	if err != nil {
		return nil, err
	}
	return segment.Update, nil
}
