// Package truehd scans raw Dolby TrueHD elementary streams without decoding
// audio. It validates access-unit framing, major-sync headers, substream
// headers, and optional substream check data.
//
// Unlike a decoder that rejects an entire malformed packet stream, Scan keeps
// complete frames before damage, separates header/payload/EOF failures, and
// searches for a trustworthy boundary after a bad header.
package truehd
