package scene

import (
	"errors"
	"strings"
	"testing"
)

func TestDuplicateJSONFieldIsRejected(t *testing.T) {
	_, err := ParseSegment([]byte(`{
		"type":"config","sequence":1,
		"layouts":[],"programs":[],
		"objects":[{"id":"a","id":"b"}]
	}`))
	if !errors.Is(err, ErrInvalidSegment) || !strings.Contains(err.Error(), "duplicate field") {
		t.Fatalf("duplicate key error = %v", err)
	}
}

func TestTrailingJSONValueIsRejected(t *testing.T) {
	_, err := ParseConfig([]byte(`{"layouts":[],"programs":[],"objects":[]} {}`))
	if !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("trailing value error = %v", err)
	}
}

func TestBareConfigAndNestedUnknownField(t *testing.T) {
	config, err := ParseConfig([]byte(`{
		"layouts":[{"id":"stereo","channels":[{"id":"L","legacyRole":3},{"id":"R"}]}],
		"programs":[],"objects":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Layouts) != 1 || config.Layouts[0].Channels[0].ID != "L" {
		t.Fatalf("config = %+v", config)
	}
	found := false
	for _, issue := range config.Unresolved {
		if issue.Path == "config.layouts[0].channels[0].legacyRole" && issue.Reason == "unknown_field" {
			found = true
		}
	}
	if !found {
		t.Fatalf("nested unknown field missing: %+v", config.Unresolved)
	}
}

func TestParseConfigRejectsUpdateEnvelope(t *testing.T) {
	_, err := ParseConfig([]byte(`{"type":"update","sequence":2,"deleteObjects":["x"]}`))
	if !errors.Is(err, ErrInvalidSegment) {
		t.Fatalf("wrong envelope error = %v", err)
	}
}
