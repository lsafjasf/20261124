// Package au parses Sun/NeXT AU audio files.
//
// Parsing is deliberately conservative. Header fields are interpreted in
// big-endian order, but byte counts in the header are never used to modify or
// extend the source buffer. Data returned by Parse always aliases the original
// bytes, and only complete sample frames that can be proven present are kept.
package au
