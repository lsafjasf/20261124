// Package fcpxml audits and conservatively repairs Final Cut Pro FCPXML.
//
// Comparison is semantic: tracks are matched by sequence/lane, clips by media
// identity, and effects by transition, timeMap, filter resource, parameters,
// and exact frame position. A conversion is compatible only when every source
// construct remains represented. Effects are never removed to obtain a pass.
package fcpxml
