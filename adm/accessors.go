package adm

import (
	"encoding/json"
	"os"
)

// PCMPath returns the immutable path of the underlying PCM map.
func (t *Transaction) PCMPath() string { return t.original.PCMPath }

// PCMSHA256 returns the expected SHA-256 of the underlying PCM map.
func (t *Transaction) PCMSHA256() string { return t.original.PCMSHA256 }

// Document returns the current working copy. Callers must not mutate it
// outside the whitelisted transaction methods.
func (t *Transaction) Document() *Document { return t.working }

// WriteDoc serializes a document to path, creating or replacing the file.
// Used for fixture creation; regular edits go through Transaction.Commit.
func WriteDoc(path string, doc *Document) error {
	return writeDoc(path, doc)
}

func writeDoc(path string, doc *Document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
