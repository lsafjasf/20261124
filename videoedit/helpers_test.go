package videoedit

import (
	"encoding/json"
	"strings"
)

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func stringsReader(text string) *strings.Reader { return strings.NewReader(text) }

type discardWriter struct{}

func (discardWriter) Write(data []byte) (int, error) { return len(data), nil }
