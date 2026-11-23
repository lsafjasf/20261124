# HDR thumbnail gamut compression

This repository replaces per-channel HDR clipping with a two-stage,
hue-preserving compressor and records unrecoverable regions for every frame.
It uses only the Go standard library.

## Why per-channel clamping is wrong

An HDR thumbnail is a linear-light `RGB` triplet. Values below zero are valid:
they describe saturated colors that lie outside the sRGB triangle after a
wide-gamut conversion. Values above one are highlights. The old code treated
both as "out of range" and clamped each channel independently:

```go
if c < 0 { c = 0 }
if c > 1 { c = 1 }
```

That destroys the relationship between channels:

- A blue-violet edge such as `(-0.10, 0.00, 1.50)` becomes `(0, 0, 1)`, so the
  negative red is turned black and the edge loses its purple hue.
- A skin highlight such as `(1.40, 0.90, 0.50)` becomes `(1, 0.90, 0.50)`;
  red is clipped while green is not, which shifts the face toward green.

## Two-stage mapping

`hdr.Compress` separates the two cases:

1. **Wide-gamut recovery.** If any component is negative, the whole triplet is
   desaturated toward its luminance:

   ```text
   c' = c + t * (Y - c),  Y = luminance(c)
   ```

   `t` is chosen so the smallest component reaches zero. Lerping toward one
   scalar scales all channel differences by the same factor, so hue and
   luminance are preserved. This is a color-space recovery, not a channel
   clamp.

2. **Highlight shoulder.** The largest component passes through a
   C1-continuous rational shoulder starting at the preset knee. The triplet is
   then scaled by the same factor, preserving chromaticity. The shoulder is
   continuous at the knee and approaches `1` asymptotically.

The output of `Compress` is always finite and inside `[0,1]^3`.

## Presets

| preset | knee | highlight detail limit |
| --- | ---: | ---: |
| `mild` | 0.92 | 8 |
| `balanced` | 0.75 | 32 |
| `strong` | 0.55 | 128 |

A lower knee folds more highlight range into the display interval. The detail
limit is the input magnitude above which highlight detail can no longer be
represented faithfully in an 8-bit thumbnail. Values above the limit are still
mapped; the frame report records where detail was lost.

## Per-frame reports

`hdr.CompressFrame` returns a new frame and a `hdr.FrameReport`. Negative
recovery and highlight compression are counted separately:

```go
out, report := hdr.CompressFrame(frame, hdr.Balanced)
```

The report contains:

- `negative_recovered`: pixels whose negative components were recovered as
  wide-gamut color.
- `highlight_compressed`: pixels whose maximum component exceeded one.
- `unrecoverable`, split into `non_finite`, `negative_luminance`, and
  `highlight_detail_lost`.
- `regions`: deterministic, 4-connected bounding boxes for each loss reason.
  Each region includes the actual pixel count, which can be smaller than the
  bounding-box area.

`hdr.ReportLog` writes one JSON object per frame (JSON Lines), assigning frame
indices in write order:

```go
log := hdr.NewReportLog(writer)
_, err := log.Write(report)
```

## Synthetic comparison

`fixtures` builds three deterministic scenes:

- `gradient`: a smooth wide-gamut/skin ramp with negative components and about
  four stops of highlights.
- `color-checker`: solid-color tiles with skin, blue-violet, magenta, cyan, and
  primary colors at low, medium, and high intensity.
- `emitters`: three glows with warm cores and negative-red blue-violet rims.

Run all three scenes against all three presets:

```sh
go test ./...
go run ./cmd/hdrcompare
```

The CLI writes `artifacts/comparison.json` and preview images under
`artifacts/comparison/`. Each `*__comparison.png` strip contains four panels:

```text
[naive clamp] [mild] [balanced] [strong]
```

The console table reports mean circular hue error in degrees for the old clamp
and the new compressor, plus negative-recovery, highlight-compression,
unrecoverable, and region counts.

## Packages

- `hdr`: color type, compressor, frame API, reports, JSON Lines logger.
- `fixtures`: deterministic synthetic HDR scenes.
- `compare`: metrics, old-clamp baseline, sRGB PNG and strip rendering.
- `cmd/hdrcompare`: comparison CLI.
