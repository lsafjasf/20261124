# 字幕强制叙述 / 完整字幕重复核对

一个仅使用 Go 标准库的审片辅助工具。输入强制叙述字幕轨、完整字幕轨以及每轨的显示区域，输出按时间、区域和轨道来源归类的候选冲突。工具不会读取或分析画面，只使用字幕文本、时间轴和配置中的显示区域。

候选不等于已确认错误。低置信度候选会明确标记 `needsManualReview: true`，并给出人工复核原因、帧级证据和建议的修复动作；工具不会修改原始字幕。

## 快速开始

```bash
go test ./...
go run ./cmd/subtitle-conflicts \
  -config examples/review.json \
  -out examples/report.json
```

标准输出是便于审阅的文本报告，`-out` 额外写入 JSON 报告：

```text
候选冲突: 3  需人工复核: 1
[conflict-001] 候选/需人工  置信度 0.706
  时间: 00:00:01,100 - 00:00:03,000  (帧 27..75 半开, 48 帧)
  区域: 交集 0.3333  [x=0.0800 y=0.8000 w=0.8400 h=0.0400]
  轨道: 强制=forced-narrative(forced.srt) cue=1  完整=full-subtitles(full.srt) cue=1
```

示例输入位于 `examples/`，生成结果位于 `examples/report.json`。

## 命令行

```bash
go run ./cmd/subtitle-conflicts -config review.json [options]
```

- `-config`：必填，核对配置 JSON。
- `-out`：可选，JSON 报告路径；先写临时文件再原子替换。
- `-format text|json`：标准输出格式，默认 `text`。
- `-fail-on-manual`：存在人工复核项时以状态码 `3` 结束，便于流水线阻断。

## 配置

坐标和尺寸均为归一化值，原点在画面左上角，范围 `[0, 1]`。单条 cue 可以在 JSON 输入中覆盖轨道默认区域。

```json
{
  "fps": 25,
  "minOverlapMs": 120,
  "candidateThreshold": 0.38,
  "minTextSimilarity": 0.34,
  "manualReviewBelow": 0.72,
  "manualRegionBelow": 0.25,
  "manualTextBelow": 0.72,
  "manualShortTextRunes": 3,
  "minRegionOverlap": 0,
  "tracks": [
    {
      "id": "forced-narrative",
      "kind": "forced_narrative",
      "source": "forced.srt",
      "language": "zh-Hans",
      "region": {"x": 0.08, "y": 0.72, "width": 0.84, "height": 0.12}
    },
    {
      "id": "full-subtitles",
      "kind": "full",
      "source": "full.srt",
      "language": "zh-Hans",
      "region": {"x": 0.08, "y": 0.80, "width": 0.84, "height": 0.12}
    }
  ]
}
```

关键阈值：

- `minOverlapMs`：两条 cue 的最小实际时间重合；接触但不重叠的半开区间不会入选。
- `minTextSimilarity`：进入候选所需的文本最低相似度。
- `candidateThreshold`：进入报告所需的综合置信度。
- `manualReviewBelow`：综合置信度低于此值即需人工判断。
- `manualTextBelow`、`manualRegionBelow`：文本或区域证据单独低于阈值时仍需人工判断。
- `manualShortTextRunes`：少于该字数的文本会降低有效文本置信度，避免“是”“好”等短句仅凭相同字符被自动认定。
- `minRegionOverlap`：可选硬门槛，按交集面积占较小区域面积的比例计算；默认 `0`，建议先保留低置信度候选。

轨道 `kind` 支持 `forced_narrative`（别名 `forced`、`forced-narrative`）和 `full`（别名 `full-subtitle`、`complete`）。

## 输入格式

### SRT

使用常规 `HH:MM:SS,mmm --> HH:MM:SS,mmm` 时间码，支持 CRLF、BOM、多行文本、HTML/ASS 样式标记。cue 结束时间按半开区间处理。

### JSON cues

文件名必须以 `.json` 结尾。既支持数组，也支持 `{"cues": [...]}`：

```json
{
  "cues": [
    {
      "id": "f-1",
      "startMs": 1000,
      "endMs": 2500,
      "text": "注意列车",
      "region": {"x": 0.1, "y": 0.7, "width": 0.8, "height": 0.2}
    }
  ]
}
```

`region` 可省略并继承轨道默认区域。cue 的 `endMs` 为排他结束时间。

## 判定模型

每条强制叙述 cue 与时间上重叠的完整字幕 cue 逐对比较，不使用简单的字符串相等判定：

1. 文本先去除 HTML/ASS 标记、全半角差异、标点、空白与大小写差异，不做翻译、分词或语义推断。
2. 文本证据由三部分组成：字符级编辑相似度、较短文本的包含度、Unicode 双字组 Dice 系数，加权为 `0.50 / 0.30 / 0.20`。
3. 少于 `manualShortTextRunes` 个字符的文本最多按 `0.55` 的文本置信度参与综合计算，并标记短文本歧义。
4. 时间证据为实际重合时长占较短 cue 时长的比例。
5. 区域证据为交集面积占较小区域面积的比例。
6. 综合置信度为 `0.55 * 文本 + 0.25 * 时间 + 0.20 * 区域`。

因此，模糊措辞、样式差异、部分时长重合都可能成为候选；完全相同但区域不重叠的内容也会保留为低置信度人工项。

## 帧级证据

帧号使用 `[startFrame, endFrameExclusive)` 半开区间。cue 起始帧为 `floor(startMs * fps / 1000)`，排他结束帧为 `ceil(endMs * fps / 1000)`。报告同时给出：

- 两条 cue 各自的绝对帧区间；
- 两者帧区间交集的首帧、排他尾帧和总帧数；
- 首帧、中间帧、尾帧上两条轨道的 `active` 状态；
- 精确到毫秒的冲突时间段。

## 修复建议

每个候选都包含两种互斥备选动作，低置信度项还会先要求人工复核：

- `manual_review`：逐帧核对文本、区域和轨道来源，不自动修改。
- `suppress_forced_narrative_overlap`：保留完整字幕，在证据帧段隐藏强制叙述；若整条 cue 都重合，可删除整条 cue。
- `suppress_full_subtitle_overlap`：若编辑策略要求保留强制叙述，则在完整字幕轨隐藏同一帧段。

所有建议均为 `automatic: false`。工具只生成报告，不直接写回字幕或视频容器。

## 边界

- 不分析画面、人脸、镜头或场景语义。
- 不做机器翻译，不判断两种语言文本是否语义相同。
- 跨多条完整字幕 cue 分散覆盖同一条强制叙述的情况，当前按逐对候选报告，人工复核时可合并处理。
- 区域来自配置或 JSON cue；SRT 本身不携带统一的屏幕坐标，因此 SRT 轨道必须配置默认区域。
