package fcpxml

import "io"

// ParseFile parses an FCPXML file and retains its original bytes.
func ParseFile(path string) (*Document, error) {
	return ParseDocumentFile(path)
}

// Parse parses an FCPXML stream.
func Parse(r io.Reader) (*Document, error) {
	return ParseDocument(r)
}

// Audit performs a static audit when candidate is nil, otherwise it compares
// source and candidate.
func Audit(source, candidate *Document) (*Report, error) {
	return AuditDocuments(source, candidate)
}

// Compare compares two parsed FCPXML documents.
func Compare(source, candidate *Document) (*Comparison, error) {
	return CompareDocuments(source, candidate)
}

// Repair returns a verified conservative repair for a candidate conversion.
func Repair(source, candidate *Document) (*RepairResult, error) {
	return RepairDocuments(source, candidate, nil)
}
