package scene

import (
	"errors"
	"testing"
)

const switchConfig = `{
	"type":"config","sequence":1,
	"layouts":[
		{"id":"5.1","channels":[{"id":"L"},{"id":"R"},{"id":"C"},{"id":"LFE"},{"id":"Ls"},{"id":"Rs"}]},
		{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]},
		{"id":"7.1","channels":[{"id":"L"},{"id":"R"},{"id":"C"},{"id":"LFE"},{"id":"Ls"},{"id":"Rs"},{"id":"Lb"},{"id":"Rb"}]}
	],
	"programs":[{"id":"main","layoutId":"5.1","objectIds":["o"]}],
	"objects":[{"id":"o","layoutChannels":{"5.1":["C"],"stereo":["L","R"]}}]
}`

func findMapping(result Result, objectID string) *ObjectChannelMapping {
	for i := range result.ObjectChannels {
		if result.ObjectChannels[i].ObjectID == objectID {
			return &result.ObjectChannels[i]
		}
	}
	return nil
}

func TestLayoutSwitchReResolvesWithoutOldFallback(t *testing.T) {
	engine := NewEngine()
	initial := applyJSON(t, engine, switchConfig)
	if initial.ObjectChannels[0].Channels[0].ID != "C" {
		t.Fatalf("initial mapping: %+v", initial.ObjectChannels)
	}

	stereo := applyJSON(t, engine, `{
		"type":"update","sequence":2,
		"programs":[{"id":"main","layoutId":"stereo"}]
	}`)
	mapping := findMapping(stereo, "o")
	if mapping == nil || mapping.LayoutID != "stereo" || len(mapping.Channels) != 2 {
		t.Fatalf("stereo mapping: %+v", stereo.ObjectChannels)
	}
	if mapping.Channels[0].ID != "L" || mapping.Channels[1].ID != "R" {
		t.Fatalf("unexpected stereo channels: %+v", mapping.Channels)
	}

	sevenOne := applyJSON(t, engine, `{
		"type":"update","sequence":3,
		"programs":[{"id":"main","layoutId":"7.1"}]
	}`)
	if findMapping(sevenOne, "o") != nil {
		t.Fatalf("object retained an old mapping after layout switch: %+v", sevenOne.ObjectChannels)
	}
	if !hasIssue(sevenOne.Unresolved, `objects["o"].layoutChannels["7.1"]`, "layout_mapping_missing") {
		t.Fatalf("missing layout-specific binding was not reported: %+v", sevenOne.Unresolved)
	}

	unknown := applyJSON(t, engine, `{
		"type":"update","sequence":4,
		"programs":[{"id":"main","layoutId":"ghost"}]
	}`)
	if findMapping(unknown, "o") != nil {
		t.Fatalf("object mapped through an unknown layout: %+v", unknown.ObjectChannels)
	}
	if !hasIssue(unknown.Unresolved, `programs["main"].layoutId`, "unknown_layout") {
		t.Fatalf("unknown layout was not reported: %+v", unknown.Unresolved)
	}

	back := applyJSON(t, engine, `{
		"type":"update","sequence":5,
		"programs":[{"id":"main","layoutId":"5.1"}]
	}`)
	if findMapping(back, "o") == nil || findMapping(back, "o").Channels[0].ID != "C" {
		t.Fatalf("explicit 5.1 binding was not restored: %+v", back.ObjectChannels)
	}
}

func TestObjectDeletionDoesNotResurrectChannels(t *testing.T) {
	engine := NewEngine()
	applyJSON(t, engine, `{
		"type":"config","sequence":1,
		"layouts":[{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]}],
		"programs":[{"id":"main","layoutId":"stereo","objectIds":["keep","gone"]}],
		"objects":[
			{"id":"keep","layoutChannels":{"stereo":["L"]}},
			{"id":"gone","layoutChannels":{"stereo":["R"]}}
		]
	}`)
	deleted := applyJSON(t, engine, `{
		"type":"update","sequence":2,
		"deleteObjects":["gone"]
	}`)
	if findMapping(deleted, "gone") != nil {
		t.Fatalf("deleted object still has a mapping: %+v", deleted.ObjectChannels)
	}
	if findMapping(deleted, "keep") == nil {
		t.Fatalf("surviving object lost its mapping: %+v", deleted.ObjectChannels)
	}
	if !hasIssue(deleted.Unresolved, `programs["main"].objectIds`, "missing_object") {
		t.Fatalf("dangling program reference was not reported: %+v", deleted.Unresolved)
	}
	foundDeleted := false
	for _, object := range deleted.Snapshot.Objects {
		if object.ID == "gone" {
			foundDeleted = true
		}
	}
	if foundDeleted {
		t.Fatal("deleted object remains in snapshot")
	}

	readded := applyJSON(t, engine, `{
		"type":"update","sequence":3,
		"objects":[{"id":"gone"}]
	}`)
	if findMapping(readded, "gone") != nil {
		t.Fatalf("empty re-add resurrected old channels: %+v", readded.ObjectChannels)
	}
	if !hasIssue(readded.Unresolved, `objects["gone"].layoutChannels["stereo"]`, "missing_channel_binding") {
		t.Fatalf("empty object binding was not reported: %+v", readded.Unresolved)
	}
}

func TestBadReferencesAreReportedIndividually(t *testing.T) {
	engine := NewEngine()
	result := applyJSON(t, engine, `{
		"type":"config","sequence":1,
		"layouts":[{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]}],
		"programs":[{"id":"main","layoutId":"stereo","objectIds":["o","o","missing"]}],
		"objects":[{"id":"o","layoutChannels":{"stereo":["L","L","X"]}}]
	}`)
	if len(result.ObjectChannels) != 1 || len(result.ObjectChannels[0].Channels) != 1 {
		t.Fatalf("unexpected mappings: %+v", result.ObjectChannels)
	}
	want := []struct {
		path   string
		reason string
	}{
		{`programs["main"].objectIds`, "duplicate_object_reference"},
		{`programs["main"].objectIds`, "missing_object"},
		{`objects["o"].layoutChannels["stereo"][1]`, "duplicate_channel_binding"},
		{`objects["o"].layoutChannels["stereo"][2]`, "unknown_output_channel"},
	}
	for _, item := range want {
		if !hasIssue(result.Unresolved, item.path, item.reason) {
			t.Errorf("missing issue path=%s reason=%s in %+v", item.path, item.reason, result.Unresolved)
		}
	}
}

func TestInvalidUpdateRollsBack(t *testing.T) {
	engine := NewEngine()
	applyJSON(t, engine, switchConfig)
	before, beforeMappings, _ := engine.Current()

	result, err := engine.ApplyJSON([]byte(`{
		"type":"update","sequence":2,
		"objects":[
			{"id":"o","layoutChannels":{"5.1":["R"]}},
			{"id":"o","layoutChannels":{"5.1":["L"]}}
		]
	}`))
	if !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("invalid update error = %v", err)
	}
	if result.Applied || result.Snapshot != nil {
		t.Fatalf("invalid update returned a snapshot: %+v", result)
	}
	after, afterMappings, _ := engine.Current()
	if len(before.Objects) != len(after.Objects) ||
		len(beforeMappings) != len(afterMappings) ||
		beforeMappings[0].Channels[0].ID != afterMappings[0].Channels[0].ID {
		t.Fatalf("state changed after rollback: before=%+v after=%+v", beforeMappings, afterMappings)
	}
}

func TestDuplicateLayoutChannelsRejectConfig(t *testing.T) {
	engine := NewEngine()
	result, err := engine.ApplyJSON([]byte(`{
		"type":"config","sequence":1,
		"layouts":[{"id":"bad","channels":[{"id":"L"},{"id":"L"}]}],
		"programs":[],"objects":[]
	}`))
	if !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("duplicate channel error = %v", err)
	}
	if result.Snapshot != nil {
		t.Fatalf("invalid config produced a snapshot: %+v", result)
	}
	if _, _, issues := engine.Current(); len(issues) != 0 {
		t.Fatalf("invalid config changed engine: %+v", issues)
	}
}

func TestAliasSequenceField(t *testing.T) {
	segment, err := ParseSegment([]byte(`{
		"type":"config","effectiveSequence":42,
		"layouts":[],"programs":[],"objects":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if segment.Sequence != 42 || !segment.HasSequence {
		t.Fatalf("unexpected sequence: %+v", segment)
	}
}
