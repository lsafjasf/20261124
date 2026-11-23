package fcpxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// ExportSupported writes the timing-bearing supported subset as canonical
// FCPXML. Unknown metadata is retained; unsupported retiming elements are
// omitted because they are outside the analyzer's 1:1 supported subset.
func (d *Document) ExportSupported(writer io.Writer) error {
	if d == nil || d.Root == nil {
		return fmt.Errorf("nil FCPXML document")
	}
	if hasRetiming(d.Root) {
		clone := d.Root.cloneFiltered()
		temp := &Document{Root: clone}
		return temp.Export(writer)
	}
	return d.Export(writer)
}

// ExportFile writes a canonical supported-subset document atomically.
func (d *Document) ExportFile(path string) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := d.ExportSupported(temp); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return fmt.Errorf("set export permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync export file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close export file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace export file: %w", err)
	}
	return nil
}

func hasRetiming(root *Node) bool {
	found := false
	_ = root.Walk(func(node *Node) error {
		switch node.Name.Local {
		case "timeMap", "timept", "conform-rate":
			found = true
		}
		return nil
	})
	return found
}

func (n *Node) cloneFiltered() *Node {
	if n == nil {
		return nil
	}
	clone := &Node{
		Name:  n.Name,
		Attrs: append([]xml.Attr(nil), n.Attrs...),
		Text:  n.Text,
	}
	for _, child := range n.Children {
		switch child.Name.Local {
		case "timeMap", "timept", "conform-rate":
			continue
		}
		filtered := child.cloneFiltered()
		filtered.parent = clone
		clone.Children = append(clone.Children, filtered)
	}
	return clone
}

// RoundTripResult captures timeline equality after export and re-import.
type RoundTripResult struct {
	Before          *Timeline
	After           *Timeline
	BeforeSignature []string
	AfterSignature  []string
}

// VerifyCutRoundTrip exports the selected sequence, parses it again, and
// verifies that every key cut frame and picture span is unchanged.
func (d *Document) VerifyCutRoundTrip(sequenceID string) (*RoundTripResult, error) {
	before, err := d.AnalyzeSequence(sequenceID)
	if err != nil {
		return nil, fmt.Errorf("analyze before round trip: %w", err)
	}
	var encoded bytes.Buffer
	if err := d.ExportSupported(&encoded); err != nil {
		return nil, err
	}
	imported, err := Parse(encoded.Bytes())
	if err != nil {
		return nil, fmt.Errorf("re-import supported export: %w", err)
	}
	after, err := imported.AnalyzeSequence(sequenceID)
	if err != nil {
		return nil, fmt.Errorf("analyze after round trip: %w", err)
	}
	result := &RoundTripResult{
		Before:          before,
		After:           after,
		BeforeSignature: before.CutSignature(),
		AfterSignature:  after.CutSignature(),
	}
	if !slices.Equal(result.BeforeSignature, result.AfterSignature) {
		return result, fmt.Errorf("round-trip changed key cuts (%d before, %d after)",
			len(result.BeforeSignature), len(result.AfterSignature))
	}
	return result, nil
}
