# WebVTT 窄屏预览（纯 Go）

这个仓库提供一个无浏览器、无 JavaScript、无第三方依赖的 WebVTT 预览器。它会把 cue
按时间排序，按 `regionanchor` / `viewportanchor` 放到 320x180 的窄屏中，限制显示行数，
检测时间与空间冲突，并输出带 cue ID 的 SVG 和 JSON。

Cue 文字始终使用固定字号（默认 16px）。冲突只被标注和报告，不会通过整体
`scale(...)` 或缩小字体来掩盖。

## 可复核产物

- `testdata/narrow.vtt`：同时包含乱序、同一时刻、1ms 相邻、负 margin、缺失样式、
  行数超限、区域越界的输入。
- `artifacts/preview.svg`：由 Go 渲染器直接生成，可浏览器打开。
- `artifacts/preview.json`：同一份布局与诊断的机器可读结果。

重新生成：

```bash
go test ./...
go run ./cmd/webvttpreview \
  -input testdata/narrow.vtt \
  -svg artifacts/preview.svg \
  -json artifacts/preview.json
```

CLI 也会打印触发规则的 ID，例如：

```text
active cue IDs: same-time-b, same-time-a
TIME_SORT        cue IDs: long-line, negative-margin, same-time-b, adjacent-next, missing-style
LINE_LIMIT       cue IDs: long-line
NEGATIVE_MARGIN  cue IDs: negative-margin
MISSING_STYLE    cue IDs: missing-style
REGION_CLAMP     cue IDs: off-edge
CONFLICT         cue IDs: same-time-b, same-time-a
```

## 规则语义

| 规则 | 触发条件 | 处理 |
| --- | --- | --- |
| `TIME_SORT` | 源文件顺序不等于时间顺序 | 按 `start, end` 稳定排序；完全相同时间保留源码顺序 |
| `REGION_CLAMP` | 锚点计算出的 region 框越过安全视口 | 夹紧到视口内，诊断列出该 region 的全部 cue ID |
| `LINE_LIMIT` | 折行数超过 `-max-lines`（默认 2） | 保留前 N 行，末行加省略号，不缩小字号 |
| `NEGATIVE_MARGIN` | `STYLE` 中任意负 margin | 布局时归零，诊断保留原始负数 |
| `MISSING_STYLE` | cue class 没有对应 `STYLE` 规则 | 使用默认颜色 `#f5f7ff` |
| `MISSING_REGION` | cue 引用了不存在的 region | 回退到底部默认 region |
| `CONFLICT` | 两个 cue 的时间区间和实际矩形同时相交 | 保留 1:1 布局，标红虚线并列出两个 cue ID |

时间活动区间按半开区间 `[start, end)` 计算，因此 `end == next.start` 或相差 1ms
都不会被误报为冲突。

## 布局说明

区域宽度是安全内容区宽度的百分比：

```text
x = safe + viewportAnchor.x * contentWidth  - regionAnchor.x * regionWidth
y = safe + viewportAnchor.y * contentHeight - regionAnchor.y * regionHeight
```

计算结果会被夹紧到安全视口。折行使用确定性 rune 宽度估算（CJK 按 1em，拉丁字符按
约 0.61em），不会调用浏览器或外部字体度量。

SVG 中每个 cue、时间轴项和诊断行都包含可机读属性，例如
`data-cue-id="same-time-a"`、`data-rules` 和
`data-issue-code="CONFLICT" data-cue-ids="same-time-b same-time-a"`。

## 代码入口

```go
doc, err := webvtt.Parse(reader)
report, err := webvtt.Analyze(doc, webvtt.DefaultOptions())
svg, err := webvtt.RenderSVG(report)
```

主要文件：

- `webvtt/parse.go`：WebVTT、REGION、STYLE、cue settings 解析。
- `webvtt/layout.go`：稳定排序、锚点定位、折行与行数限制。
- `webvtt/analyze.go`：规则触发、冲突检测、报告生成。
- `webvtt/svg.go`、`webvtt/svg_extra.go`：纯 Go SVG 渲染。
- `cmd/webvttpreview/main.go`：CLI。

## 测试

```bash
go test ./...
```

四个指定场景分别对应：

- `TestSameTimestampConflictKeepsSourceOrder`
- `TestAdjacentMillisecondCuesDoNotConflict`
- `TestNegativeMarginIsClampedAndReported`
- `TestMissingStyleFallsBackAndReportsCueID`

另外覆盖了乱序排序、锚点位置与越界夹紧、行数截断、SVG 固定字号/确定性，以及 CLI
同时产出 SVG 和 JSON。
