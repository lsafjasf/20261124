// Package fcpxml parses, validates, and flattens Final Cut Pro FCPXML
// timelines using only the Go standard library.
//
// Times remain exact rational seconds while nested sequence offsets are
// composed. Analyze converts visible picture leaves to zero-based top-sequence
// frames, and VerifyCutRoundTrip checks that export and re-import preserve the
// resulting cut signature.
package fcpxml
