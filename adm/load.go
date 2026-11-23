package adm

import (
	"fmt"
	"os"
)

// Load opens path read-only and parses RIFF, chna and axml. The file is
// never modified; sample data is never decoded.
func Load(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	riff, err := ParseRIFF(fh)
	if err != nil {
		return nil, err
	}
	f := &File{RIFF: riff}
	if raw, err := riff.ReadChunk(fh, "chna"); err == nil {
		c, err := ParseChna(raw)
		if err != nil {
			return nil, fmt.Errorf("parsing chna: %w", err)
		}
		f.Chna = c
	}
	if raw, err := riff.ReadChunk(fh, "axml"); err == nil {
		d, err := ParseXML(raw)
		if err != nil {
			return nil, fmt.Errorf("parsing axml: %w", err)
		}
		f.Doc = d
	}
	return f, nil
}
