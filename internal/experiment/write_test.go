package experiment

import (
	"bytes"
	"testing"
)

func TestTextOutputsAreDeterministic(t *testing.T) {
	first, err := Run(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	writers := []struct {
		name string
		fn   func(*bytes.Buffer, *Result) error
	}{
		{"json", func(b *bytes.Buffer, r *Result) error { return WriteJSON(b, r) }},
		{"pixels", func(b *bytes.Buffer, r *Result) error { return WritePixelsCSV(b, r) }},
		{"coordinates", func(b *bytes.Buffer, r *Result) error { return WriteCoordinatesCSV(b, r) }},
		{"blocks", func(b *bytes.Buffer, r *Result) error { return WriteBlocksCSV(b, r) }},
	}
	for _, tt := range writers {
		var a, b bytes.Buffer
		if err := tt.fn(&a, first); err != nil {
			t.Fatalf("%s first run: %v", tt.name, err)
		}
		if err := tt.fn(&b, second); err != nil {
			t.Fatalf("%s second run: %v", tt.name, err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatalf("%s output changed between identical runs", tt.name)
		}
	}
}

func TestReportRecordsNoDesaturationPolicy(t *testing.T) {
	r, err := Run(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	report := NewReport(r)
	if report.Policy.GlobalDesaturation {
		t.Fatal("report policy enables global desaturation")
	}
	if report.Policy.LumaModified {
		t.Fatal("report policy modifies luma")
	}
	if report.InputHash == "" || report.RefHash == "" {
		t.Fatal("report is missing deterministic input hashes")
	}
}
