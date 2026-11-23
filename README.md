# OTIO 子集剪辑工程导出器

纯 Go、仅使用标准库的 OpenTimelineIO JSON 子集导入/编辑/再导出器，不依赖
`opentimelineio` 包。

支持的结构是明确且封闭的：

- `Timeline.1` -> `Stack.1` -> 多轨 `Track.1`
- Track 子项：`Clip.2`、`Gap.1`
- `source_range` / `TimeRange.1` / `RationalTime.1`
- `Marker.1` / `Marker.2`，支持 `marked_range` 和旧版 `range`
- marker 可挂在 timeline、stack、track、clip 或 gap 上，允许任意重叠

媒体引用（`media_reference` / `media_references`）和 `effects` 不参与结构编辑
（媒体引用仅用于缺失检测），但不会被丢弃：它们作为未知字段或未知对象原样
保留，并在报告中点名。

## 无损规则

- JSON 数字使用 `json.Number` 解码；未知数字不会被重新格式化为浮点数。
- 每个已知对象的未知字段进入 `Extra`，导出时写回原处。
- 未知 `OTIO_SCHEMA` 子对象进入 `Opaque`，整个对象树原样保留。
- `RationalTime` 的 `rate` 不会被归一化；跨帧率编辑按秒换算，并保留原
  start/duration 的各自帧率。
- 报告中的每个未知项都有 `action`，当前策略全部是 `retained`。不会静默丢弃
  未知字段。
- 缺失媒体是合法状态：导入成功，`MissingMedia` 报告中给出 JSON Pointer 和
  clip 名称，媒体字段仍原样往返。

示例报告：

```json
{
  "unknown_fields": [
    {
      "path": "/tracks/children/0/children/0/media_reference",
      "field": "media_reference",
      "action": "retained",
      "reason": "field is outside the supported subset and was retained"
    }
  ],
  "missing_media": [
    {
      "path": "/tracks/children/0/children/0",
      "clip": "no-reference",
      "reason": "clip has no media_reference or media_references field"
    }
  ]
}
```

## CLI

```bash
go run ./cmd/otioedit inspect -in testdata/missing_media.json

go run ./cmd/otioedit roundtrip \
  -in testdata/cross_frame_rate.json \
  -out /tmp/cross-frame-rate.json \
  -report /tmp/report.json

go run ./cmd/otioedit edit \
  -in testdata/overlapping_markers.json \
  -out /tmp/edited.json \
  -trim-clip marked-shot -trim-start 250ms \
  -marker-track V2 -marker-name new-note \
  -marker-start 1s -marker-duration 500ms -marker-rate 30
```

`-report -` 把完整 JSON 报告写到 stdout。`edit` 还支持 `-add-track`、
`-gap-track`、`-gap-name`、`-gap-duration` 和 `-gap-rate`。

## Go API

```go
timeline, report, err := otio.Load("project.otio")
if err != nil {
    return err
}

clip := timeline.ClipByName("film-at-24")
if clip != nil {
    if err := clip.TrimSourceStart(500 * time.Millisecond); err != nil {
        return err
    }
}

track := otio.NewTrack("A1")
music := otio.NewClip("music", otio.NewTimeRange(
    otio.NewRationalTime(30, 60),
    otio.NewRationalTime(30, 150),
))
if err := track.Append(music); err != nil {
    return err
}
track.AddMarker(otio.NewMarker("cue", "RED", otio.NewTimeRange(
    otio.NewRationalTime(24, 24),
    otio.NewRationalTime(24, 12),
)))
if err := timeline.AppendTrack(track); err != nil {
    return err
}

exportReport, err := otio.Save("edited.otio", timeline)
```

`otio.Encode` 返回 JSON 字节和当前模型报告；`otio.Analyze` 可单独扫描未知字段、
不透明对象和缺失媒体。`Report.Summary` 提供一行摘要。

## 测试夹具

- `testdata/cross_frame_rate.json`：23.976 / 24 / 25 / 29.97 / 30 fps 混用，
  覆盖多轨、gap、clip、source range 和 marker；测试会做 trim 后再次往返。
- `testdata/overlapping_markers.json`：重叠 marker、接触但不相交的 marker，
  以及跨帧率 marker range。
- `testdata/missing_media.json`：无媒体字段、null 引用、空 `target_url`、空
  `media_references` 和可用媒体引用。
- `testdata/unknown_fields.json`：时间线、clip、time range 的未知字段，以及
  `Transition.1` 不透明对象。

运行：

```bash
go test ./...
```
