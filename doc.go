// Package otio imports, edits, and re-exports a deliberately small subset of
// OpenTimelineIO JSON: a Timeline with a Stack, multiple Tracks, Clips, Gaps,
// source ranges, and markers.
//
// Fields and object schemas outside that subset are retained in Extra or
// Opaque values and named by Report. Media references are not interpreted, but
// clips without usable media are listed in Report.MissingMedia. The package
// uses only the Go standard library.
package otio
