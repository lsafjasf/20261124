# FCPXML nested timeline mapper

This repository contains a small Go library and CLI for flattening the final
picture events of an FCPXML project whose sequences contain nested compound
clips, transitions, and negative offsets.

The implementation uses only the Go standard library. In particular, FCPXML is
read and written with `encoding/xml`, and files are handled with `os` and
`path/filepath`.

## What it does

- Parses FCPXML without third-party packages and keeps element/attribute order.
- Represents all FCPXML times as exact rational seconds (`1001/30000s`).
- Builds a parent/child mapping with this rule:

  ```text
  topTime = nestedLocalTime + accumulated(offset - start)
  topFrame = round(topTime - topTcStart) / topFrameDuration
  ```

- Clips every event against each containing clip/sequence window, so negative
  offsets are resolved instead of silently dropped.
- Reports duplicate IDs, unresolved `ref`/`format`/`dataLocator` references,
  missing required references, and circular sequence nesting.
- Treats `transition.offset` as the transition interval start. The editorial
  cut is inferred from the previous picture end or next picture start, so an
  off-centre transition does not move its cut to the interval midpoint.
- Exports canonical FCPXML to a temporary file and atomically replaces the
  destination. `VerifyCutRoundTrip` re-imports the result and compares every
  picture span, exact time, and key cut frame.

Top-level picture frames are zero-based relative to the top sequence's
`tcStart`. Nesting is mapped 1:1 in time; speed remapping (`timeMap` or
`conform-rate`) is outside the supported subset and produces a warning.

## CLI

```sh
go run ./cmd/fcpxmlmap analyze testdata/nested.fcpxml
go run ./cmd/fcpxmlmap roundtrip testdata/nested.fcpxml
go run ./cmd/fcpxmlmap export testdata/nested.fcpxml /tmp/normalized.fcpxml
```

Select a sequence explicitly when a document has more than one project:

```sh
go run ./cmd/fcpxmlmap analyze -sequence sqTop project.fcpxml
```

`export` verifies round-trip cut preservation before writing by default. Use
`-verify=false` only when that check is intentionally not required.

## Library

```go
import fcpxml "fcpxmlmap"

document, err := fcpxml.Load("project.fcpxml")
timeline, err := document.Analyze()

for _, event := range timeline.PictureEvents {
    fmt.Println(event.Name, event.StartFrame, event.EndFrame)
}
for _, cut := range timeline.Cuts {
    fmt.Println(cut.Frame, cut.Time, cut.Kind)
}

if _, err := document.VerifyCutRoundTrip(""); err != nil {
    // Key cuts changed or the document is invalid.
}
```

Useful entry points:

- `Load`, `Parse`, `Document.Validate`
- `Document.Analyze`, `Document.AnalyzeSequence`
- `Timeline.PictureEvents`, `Timeline.Transitions`, `Timeline.Cuts`
- `Timeline.KeyFrames`, `Timeline.CutSignature`
- `Document.Export`, `Document.ExportSupported`, `Document.ExportFile`
- `Document.VerifyCutRoundTrip`

## Supported subset

Supported picture story elements include `asset-clip`, `video`, `clip`,
`ref-clip`, `title`, and `caption`, including nested `<sequence>` and nested
`<spine>` containers. Gaps are skipped as picture but still occupy time.
Audio-only leaves do not produce picture events.

Current limitations:

- Retiming via `timeMap`/`conform-rate` is not applied.
- Multicam angle selection and audition alternatives are not resolved.
- Audio final-mix events are intentionally excluded.
- Frame values are zero-based; they are not timecode strings.

## Verification

```sh
go test ./...
go vet ./...
```

The fixture `testdata/nested.fcpxml` combines a non-zero top timecode, 30 fps
top sequence, 24 fps nested sequences, a transition, and a nested event whose
mapped raw start is negative. Tests assert the exact top frames and that the
export/re-import cut signature is unchanged.
