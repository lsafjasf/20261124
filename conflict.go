package timeline

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// MergeResult contains the partially merged timeline and every unresolved
// conflict. A non-empty Conflicts slice means Timeline is not final.
type MergeResult struct {
	Timeline  Timeline   `json:"timeline"`
	Conflicts []Conflict `json:"conflicts"`
}

// Clean reports whether every change could be merged automatically.
func (r MergeResult) Clean() bool {
	return len(r.Conflicts) == 0
}

// Conflict identifies one property whose ancestor, ours, and theirs values
// cannot be combined automatically.
type Conflict struct {
	Path     string        `json:"path"`
	Ancestor ConflictValue `json:"ancestor"`
	Ours     ConflictValue `json:"ours"`
	Theirs   ConflictValue `json:"theirs"`
}

// ConflictValue preserves missing-vs-null because deletion and JSON null are
// different timeline edits.
type ConflictValue struct {
	Present bool `json:"present"`
	Value   any  `json:"value,omitempty"`
}

// Display renders a value for a side-by-side conflict report.
func (v ConflictValue) Display() string {
	if !v.Present {
		return "<absent>"
	}
	data, err := json.Marshal(v.Value)
	if err != nil {
		return fmt.Sprintf("<unprintable: %v>", err)
	}
	return string(data)
}

// FormatConflicts writes an aligned three-column report. Each property shows
// the ancestor and both independently edited values; no side is selected.
func FormatConflicts(w io.Writer, conflicts []Conflict) error {
	if len(conflicts) == 0 {
		_, err := fmt.Fprintln(w, "No conflicts.")
		return err
	}

	for i, conflict := range conflicts {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "CONFLICT %s\n", displayPath(conflict.Path)); err != nil {
			return err
		}
		rows := []struct {
			label string
			value string
		}{
			{"ancestor", conflict.Ancestor.Display()},
			{"ours", conflict.Ours.Display()},
			{"theirs", conflict.Theirs.Display()},
		}
		width := 0
		for _, row := range rows {
			if len(row.label) > width {
				width = len(row.label)
			}
		}
		for _, row := range rows {
			if _, err := fmt.Fprintf(w, "  %-*s : %s\n", width, row.label, row.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func displayPath(path string) string {
	if path == "" {
		return "<root>"
	}
	return strings.TrimPrefix(path, ".")
}

// Format writes the merged timeline followed by any three-way conflicts.
func (r MergeResult) Format(w io.Writer) error {
	data, err := json.MarshalIndent(r.Timeline, "", "  ")
	if err != nil {
		return fmt.Errorf("format merged timeline: %w", err)
	}
	if _, err := fmt.Fprintln(w, "MERGED TIMELINE"); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if len(r.Conflicts) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return FormatConflicts(w, r.Conflicts)
}
