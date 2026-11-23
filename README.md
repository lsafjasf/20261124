# Mixed-FPS Timeline Importer

一个仅使用 Go 标准库的时间线导入/导出工具，用来复现并修复“gap 被算成零长度、
变速片段后逐切点漂移”的导入问题。样本序列为 25 fps，素材同时包含
`24000/1001`（23.976）、`25`、`30` fps，并穿插 2x、0.5x、1.5x 变速。

## 三个时间域

- **媒体时间（media time）**：素材绝对时间。由 `mediaInFrame/mediaFPS` 和
  `mediaOutFrame/mediaFPS` 得到，与它在节目中的位置无关。
- **源时间（source time）**：片段内相对时间，从 media in 起算，范围是
  `0..(mediaOutFrame-mediaInFrame)/mediaFPS`。
- **序列时间（sequence time）**：节目时间。片段序列时长先由源时长除以 speed，
  再与 gap 的序列时长累加；gap 没有媒体/源区间。

所有时间在导入和累加阶段都是精确有理数，不使用 `float64`。只有把切点物化到
序列帧时才做一次最近帧取整，因此误差不会沿后续切点传播。

## 运行

```sh
go run ./cmd/timeline-import \
  -in testdata/mixed-fps.json \
  -roundtrip artifacts/roundtrip.json \
  -errors artifacts/cut-errors.csv \
  -summary artifacts/summary.json

go test ./...
```

输入中的 `speed` 表示 `源时间 / 序列时间`，例如 `2` 表示 2 倍速，序列时长减半。
`23.976` 和 `23.98` 会规范化为 `24000/1001`，避免把近似小数带入换算。

## 验证结果

- 理想总时长：`144.183333333 s` = `3604.583333333` 个 25 fps 序列帧。
- 修复后逐切点最大误差：`0.416666667` 帧，即 `0.016666666680 s`，只是单次
  最近帧量化误差。
- 修复后完整往返：每个媒体/源/序列时间均精确相等，最大回读误差 `0 s`。
- 旧逻辑（gap=0，且逐段取整后累加）：片尾 `3568` 帧，比理想值少
  `36.583333333` 帧，即 `1.463333333333 s`。

逐切点明细见 `artifacts/cut-errors.csv`，规范化且可重新导入的文件见
`artifacts/roundtrip.json`，机器可读汇总见 `artifacts/summary.json`。

## 根因与修复

旧逻辑在 gap 上查找媒体区间。gap 本来就没有媒体，`mediaOut-mediaIn` 得到 0，
所以 gap 被算成零长度。同时，变速片段可能产生非整数序列帧时长；若每隔一段就
取整并写回游标，这次取整会污染之后所有切点。两者叠加后，漂移在片尾最明显。

修复把三个域拆开：

```text
sourceDuration    = (mediaOutFrame - mediaInFrame) / mediaFPS
clipSeqDuration   = sourceDuration / speed
gapSeqDuration    = gapSequenceFrames / sequenceFPS
sequenceCursor    = exactSequenceCursor + eventSeqDuration
```

`sequenceCursor` 直到输出切点时才取整。相邻事件共享同一精确边界，所以量化后也不会
出现片段重叠或空洞。

## 为什么修总时长不够

总时长只是一个标量约束，不能证明或恢复每个内部切点。旧的片尾少了 36.58 帧时，
给尾部补回 36.58 帧可以把总长“修对”，但前面的 gap 仍然是 0，变速片段后的各个
source/record 映射仍然错位；这些错误还可能被尾部补偿放大。反过来，按总时长做
整体缩放虽能对齐首尾，却会改变每一段的速度语义，往返时无法还原原始媒体帧。

正确性约束是逐事件、逐边界的：

```text
media range -> source duration -> speed -> sequence duration -> exact cursor
```

`cut-errors.csv` 同时列出修复值和旧逻辑值，可以看到修复值始终在 ±0.5 序列帧内，
而旧值从第一个 gap 起逐级变负。真正需要回读验证的是所有切点和媒体映射，而不是
仅比较一个 duration 数字。
