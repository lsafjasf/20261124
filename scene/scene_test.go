package scene

import (
	"errors"
	"testing"
)

func applyJSON(t *testing.T, engine *Engine, input string) Result {
	t.Helper()
	result, err := engine.ApplyJSON([]byte(input))
	if err != nil {
		t.Fatalf("ApplyJSON() error = %v; result = %+v", err, result)
	}
	return result
}

func hasIssue(issues []UnresolvedField, path, reason string) bool {
	for _, issue := range issues {
		if issue.Path == path && issue.Reason == reason {
			return true
		}
	}
	return false
}

func TestConfigResolvesObjectsToOutputChannels(t *testing.T) {
	engine := NewEngine()
	result := applyJSON(t, engine, `{
		"type": "config",
		"sequence": 10,
		"config": {
			"sceneId": "demo",
			"layouts": [{
				"id": "5.1",
				"channels": [
					{"id": "L"}, {"id": "R"}, {"id": "C"},
					{"id": "LFE"}, {"id": "Ls"}, {"id": "Rs"}
				]
			}],
			"programs": [{
				"id": "main", "layoutId": "5.1",
				"objectIds": ["voice", "music"]
			}],
			"objects": [
				{"id": "voice", "layoutChannels": {"5.1": ["C"]}},
				{"id": "music", "channels": ["L", "R"]}
			],
			"vendorHint": true,
			"extensions": {"x-vendor/private": {"value": 7}}
		}
	}`)

	if !result.Applied || result.EffectiveSequence != 10 || result.Revision != 1 {
		t.Fatalf("unexpected result envelope: %+v", result)
	}
	if len(result.ObjectChannels) != 2 {
		t.Fatalf("got %d mappings, want 2: %+v", len(result.ObjectChannels), result.ObjectChannels)
	}
	voice := result.ObjectChannels[0]
	if voice.ObjectID != "voice" || voice.ProgramID != "main" || voice.LayoutID != "5.1" {
		t.Fatalf("unexpected voice mapping: %+v", voice)
	}
	if len(voice.Channels) != 1 || voice.Channels[0] != (OutputChannel{ID: "C", Index: 2}) {
		t.Fatalf("unexpected voice channels: %+v", voice.Channels)
	}
	music := result.ObjectChannels[1]
	if len(music.Channels) != 2 ||
		music.Channels[0] != (OutputChannel{ID: "L", Index: 0}) ||
		music.Channels[1] != (OutputChannel{ID: "R", Index: 1}) {
		t.Fatalf("unexpected music channels: %+v", music.Channels)
	}
	if !hasIssue(result.Unresolved, "config.vendorHint", "unknown_field") {
		t.Fatalf("missing unknown field report: %+v", result.Unresolved)
	}
	if !hasIssue(result.Unresolved, "extensions.x-vendor/private", "unknown_extension") {
		t.Fatalf("missing extension report: %+v", result.Unresolved)
	}
}

func TestUnknownExtensionPersistsAcrossUpdates(t *testing.T) {
	engine := NewEngine()
	applyJSON(t, engine, `{
		"type":"config","sequence":1,
		"layouts":[{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]}],
		"programs":[{"id":"main","layoutId":"stereo","objectIds":["o"]}],
		"objects":[{"id":"o","layoutChannels":{"stereo":["L"]}}],
		"extensions":{"x-unknown":[1,2,3]}
	}`)
	result := applyJSON(t, engine, `{
		"type":"update","sequence":2,
		"objects":[{"id":"o","layoutChannels":{"stereo":["R"]}}]
	}`)
	if !hasIssue(result.Unresolved, "extensions.x-unknown", "unknown_extension") {
		t.Fatalf("extension was not reported after update: %+v", result.Unresolved)
	}
	if result.ObjectChannels[0].Channels[0] != (OutputChannel{ID: "R", Index: 1}) {
		t.Fatalf("unexpected mapping: %+v", result.ObjectChannels)
	}
}

func TestSequenceFailuresDoNotReturnOldSnapshot(t *testing.T) {
	engine := NewEngine()
	applyJSON(t, engine, `{
		"type":"config","sequence":10,
		"layouts":[{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]}],
		"programs":[{"id":"main","layoutId":"stereo","objectIds":["o"]}],
		"objects":[{"id":"o","layoutChannels":{"stereo":["L"]}}]
	}`)
	before, _, _ := engine.Current()

	duplicate, err := engine.ApplyJSON([]byte(`{"type":"update","sequence":10,"objects":[{"id":"o","layoutChannels":{"stereo":["R"]}}]}`))
	if !errors.Is(err, ErrDuplicateSequence) {
		t.Fatalf("duplicate sequence error = %v", err)
	}
	if duplicate.Applied || duplicate.Snapshot != nil {
		t.Fatalf("duplicate returned a snapshot: %+v", duplicate)
	}
	if duplicate.EffectiveSequence != 10 || !hasIssue(duplicate.Unresolved, "sequence", "duplicate_sequence") {
		t.Fatalf("duplicate report is incomplete: %+v", duplicate)
	}

	stale, err := engine.ApplyJSON([]byte(`{"type":"update","sequence":9,"objects":[{"id":"o","layoutChannels":{"stereo":["R"]}}]}`))
	if !errors.Is(err, ErrStaleSequence) {
		t.Fatalf("stale sequence error = %v", err)
	}
	if stale.Applied || stale.Snapshot != nil || stale.EffectiveSequence != 9 {
		t.Fatalf("stale update was applied: %+v", stale)
	}

	after, mappings, _ := engine.Current()
	if before.SceneID != after.SceneID || len(mappings) != 1 || mappings[0].Channels[0].ID != "L" {
		t.Fatalf("state changed after rejected updates: before=%+v after=%+v mappings=%+v", before, after, mappings)
	}
}

func TestMissingSequenceAndMissingConfig(t *testing.T) {
	engine := NewEngine()
	missing, err := engine.ApplyJSON([]byte(`{"type":"update","objects":[]}`))
	if !errors.Is(err, ErrMissingSequence) {
		t.Fatalf("missing sequence error = %v", err)
	}
	if !hasIssue(missing.Unresolved, "sequence", "missing_sequence") {
		t.Fatalf("missing sequence report: %+v", missing)
	}

	noConfig, err := engine.ApplyJSON([]byte(`{"type":"update","sequence":1,"objects":[]}`))
	if !errors.Is(err, ErrNoConfiguration) {
		t.Fatalf("missing config error = %v", err)
	}
	if noConfig.Applied || noConfig.Snapshot != nil {
		t.Fatalf("update without config produced a snapshot: %+v", noConfig)
	}
}

func TestInlineConfigParsing(t *testing.T) {
	engine := NewEngine()
	result := applyJSON(t, engine, `{
		"type":"config","sequence":1,
		"layouts":[{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]}],
		"programs":[{"id":"main","layoutId":"stereo","objectIds":["o"]}],
		"objects":[{"id":"o","layoutChannels":{"stereo":["R"]}}]
	}`)
	if len(result.ObjectChannels) != 1 || result.ObjectChannels[0].Channels[0].Index != 1 {
		t.Fatalf("unexpected inline mapping: %+v", result.ObjectChannels)
	}
}
