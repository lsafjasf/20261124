package scene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ProcessStream applies a JSON array of segments or a whitespace-delimited
// JSON stream (one segment after another). Parsing or sequence failures are
// retained as Results and joined into the returned error; later segments are
// still attempted.
func (e *Engine) ProcessStream(raw []byte) ([]Result, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty stream", ErrInvalidSegment)
	}
	var segments []json.RawMessage
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &segments); err != nil {
			return nil, fmt.Errorf("%w: segment array: %v", ErrInvalidSegment, err)
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(raw))
		for {
			var segment json.RawMessage
			err := dec.Decode(&segment)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%w: segment stream: %v", ErrInvalidSegment, err)
			}
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("%w: stream has no segments", ErrInvalidSegment)
	}

	results := make([]Result, 0, len(segments))
	var failures []error
	for _, rawSegment := range segments {
		result, err := e.ApplyJSON(rawSegment)
		results = append(results, result)
		if err != nil {
			failures = append(failures, err)
		}
	}
	return results, errors.Join(failures...)
}
