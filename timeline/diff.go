package timeline

import (
	"fmt"
	"reflect"
	"sort"
)

// Difference identifies either the first replay step whose post-state hash
// differs, or the first field that differs when two final projects are
// compared. Step is zero for a final-state field comparison.
type Difference struct {
	Step         uint64 `json:"step,omitempty"`
	EventID      string `json:"event_id,omitempty"`
	Path         string `json:"path,omitempty"`
	Reason       string `json:"reason"`
	Expected     any    `json:"expected,omitempty"`
	Actual       any    `json:"actual,omitempty"`
	ExpectedHash string `json:"expected_hash,omitempty"`
	ActualHash   string `json:"actual_hash,omitempty"`
}

func (d *Difference) Error() string {
	switch {
	case d.Step != 0 && d.Path != "":
		return fmt.Sprintf("replay step %d (%s): %s at %s", d.Step, d.EventID, d.Reason, d.Path)
	case d.Step != 0:
		return fmt.Sprintf("replay step %d (%s): %s", d.Step, d.EventID, d.Reason)
	case d.Path != "":
		return fmt.Sprintf("%s at %s", d.Reason, d.Path)
	default:
		return d.Reason
	}
}

// DiffProjects normalizes both projects and returns their first differing
// field in deterministic traversal order.
func DiffProjects(expected, actual Project) *Difference {
	want := expected.Normalize()
	got := actual.Normalize()
	return firstDifference(reflect.ValueOf(want), reflect.ValueOf(got), "")
}

// FirstDifference compares arbitrary JSON-like values field by field. It is
// exported for callers that store snapshots in a different container type.
func FirstDifference(expected, actual any) *Difference {
	return firstDifference(reflect.ValueOf(expected), reflect.ValueOf(actual), "")
}

func firstDifference(want, got reflect.Value, path string) *Difference {
	if !want.IsValid() || !got.IsValid() {
		if want.IsValid() == got.IsValid() {
			return nil
		}
		return newFieldDifference(path, valueInterface(want), valueInterface(got))
	}
	if want.Type() != got.Type() {
		return fieldDifference(path, "type mismatch", want, got)
	}
	if want.Kind() == reflect.Interface {
		if want.IsNil() || got.IsNil() {
			if want.IsNil() && got.IsNil() {
				return nil
			}
			return fieldDifference(path, "nil mismatch", want, got)
		}
		return firstDifference(want.Elem(), got.Elem(), path)
	}
	if want.Kind() == reflect.Pointer {
		if want.IsNil() || got.IsNil() {
			if want.IsNil() && got.IsNil() {
				return nil
			}
			return fieldDifference(path, "nil mismatch", want, got)
		}
		return firstDifference(want.Elem(), got.Elem(), path)
	}

	switch want.Kind() {
	case reflect.Bool:
		if want.Bool() != got.Bool() {
			return fieldDifference(path, "field mismatch", want, got)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if want.Int() != got.Int() {
			return fieldDifference(path, "field mismatch", want, got)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if want.Uint() != got.Uint() {
			return fieldDifference(path, "field mismatch", want, got)
		}
	case reflect.Float32, reflect.Float64:
		if want.Float() != got.Float() {
			return fieldDifference(path, "field mismatch", want, got)
		}
	case reflect.String:
		if want.String() != got.String() {
			return fieldDifference(path, "field mismatch", want, got)
		}
	case reflect.Struct:
		for i := 0; i < want.NumField(); i++ {
			field := want.Type().Field(i)
			childPath := joinPath(path, field.Name)
			if diff := firstDifference(want.Field(i), got.Field(i), childPath); diff != nil {
				return diff
			}
		}
	case reflect.Slice, reflect.Array:
		if want.Len() != got.Len() {
			return fieldDifference(path, "length mismatch", want, got)
		}
		for i := 0; i < want.Len(); i++ {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			if diff := firstDifference(want.Index(i), got.Index(i), childPath); diff != nil {
				return diff
			}
		}
	case reflect.Map:
		if want.IsNil() || got.IsNil() {
			if want.IsNil() && got.IsNil() {
				return nil
			}
			return fieldDifference(path, "nil mismatch", want, got)
		}
		wantKeys := sortedMapKeys(want)
		gotKeys := sortedMapKeys(got)
		if !equalStringSlices(wantKeys, gotKeys) {
			return fieldDifference(path, "map keys mismatch", want, got)
		}
		for _, key := range wantKeys {
			keyValue := reflect.ValueOf(key).Convert(want.Type().Key())
			childPath := fmt.Sprintf("%s[%q]", path, key)
			if diff := firstDifference(want.MapIndex(keyValue), got.MapIndex(keyValue), childPath); diff != nil {
				return diff
			}
		}
	default:
		if !reflect.DeepEqual(valueInterface(want), valueInterface(got)) {
			return fieldDifference(path, "value mismatch", want, got)
		}
	}
	return nil
}

func newFieldDifference(path string, want, got any) *Difference {
	return &Difference{Path: path, Reason: "field mismatch", Expected: want, Actual: got}
}

func fieldDifference(path, reason string, want, got reflect.Value) *Difference {
	return &Difference{
		Path:     path,
		Reason:   reason,
		Expected: valueInterface(want),
		Actual:   valueInterface(got),
	}
}

func valueInterface(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface && value.IsNil() {
		return nil
	}
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Map ||
		value.Kind() == reflect.Slice || value.Kind() == reflect.Func) && value.IsNil() {
		return nil
	}
	if value.CanInterface() {
		return value.Interface()
	}
	return nil
}

func sortedMapKeys(value reflect.Value) []string {
	keys := make([]string, 0, value.Len())
	iter := value.MapRange()
	for iter.Next() {
		keys = append(keys, fmt.Sprint(iter.Key().Interface()))
	}
	sort.Strings(keys)
	return keys
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func joinPath(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
}
