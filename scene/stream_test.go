package scene

import (
	"errors"
	"testing"
)

func TestProcessStreamArrayAndJSONL(t *testing.T) {
	const config = `{
		"type":"config","sequence":1,
		"layouts":[{"id":"stereo","channels":[{"id":"L"},{"id":"R"}]}],
		"programs":[{"id":"main","layoutId":"stereo","objectIds":["o"]}],
		"objects":[{"id":"o","layoutChannels":{"stereo":["L"]}}]
	}`
	const update = `{"type":"update","sequence":2,"objects":[{"id":"o","layoutChannels":{"stereo":["R"]}}]}`

	for name, input := range map[string]string{
		"array": "[" + config + "," + update + "]",
		"jsonl": config + "\n" + update + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			engine := NewEngine()
			results, err := engine.ProcessStream([]byte(input))
			if err != nil {
				t.Fatalf("ProcessStream() error = %v", err)
			}
			if len(results) != 2 {
				t.Fatalf("got %d results, want 2", len(results))
			}
			if results[0].EffectiveSequence != 1 || results[1].EffectiveSequence != 2 {
				t.Fatalf("sequences: %+v", results)
			}
			if results[1].ObjectChannels[0].Channels[0].ID != "R" {
				t.Fatalf("final mapping: %+v", results[1].ObjectChannels)
			}
		})
	}
}

func TestProcessStreamContinuesAfterRejectedUpdate(t *testing.T) {
	engine := NewEngine()
	results, err := engine.ProcessStream([]byte(`
		{"type":"config","sequence":1,"layouts":[],"programs":[],"objects":[]}
		{"type":"update","sequence":1,"layouts":[]}
		{"type":"update","sequence":2,"layouts":[]}
	`))
	if !errors.Is(err, ErrDuplicateSequence) {
		t.Fatalf("error = %v", err)
	}
	if len(results) != 3 || results[1].Applied || !results[2].Applied {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[1].EffectiveSequence != 1 || results[2].EffectiveSequence != 2 {
		t.Fatalf("unexpected effective sequences: %+v", results)
	}
}
