# 20261124

Pure-Go (stdlib only) HDR->SDR pipeline that reproduces, diagnoses, and fixes
a double tone-mapping defect on bright shots. No external media tools.

- `go run . report` — full investigation, writes `POSTMORTEM.md`
- `go run . stages` — per-stage back-solved curve fits for every frame
- `go test ./...` — golden comparison vs known single-tone-map result

Findings: the second tone map is applied by the `look` module
(`tonemap -> look` boundary) on frames 2-4, gated by the auto-enabled
`HighlightProtect` parameter. The postmortem proves the cause is not the
exposure adjustment: the exposure stage fits a pure linear gain to machine
precision, the defect fits `tonemap^2` exactly, and no exposure gain can
reproduce the artifact.
