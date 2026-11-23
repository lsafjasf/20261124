# 29.97 DF/NDF EDL 逐帧时间线

纯 Go 标准库实现的 CMX 3600 EDL 解析、逐帧时间线展开和只保留切点的导回工具。
输入可批量混放 `FCM: DROP FRAME` 与 `FCM: NON-DROP FRAME` 文件；帧率以
`30000/1001` 精确有理数保存，不使用 `float64` 做帧号换算。

## 样例结果

`testdata/` 中的三个 EDL 覆盖 DF 跨午夜、NDF 同卷复用、负源/记录偏移和显式负时间码。

| 文件 | 模式 | 首切点（相对/绝对帧） | 尾切点（相对/绝对帧） | 总帧数 | 时长 | 同卷复用 | 无法保留的转场 |
|---|---|---:|---:|---:|---:|---:|---|
| `df_midnight.edl` | DF | `0 / 2589108` | `1200 / 2590308` | 1200 | 40.04 s | `TAPE_A` x2 | 1 个 dissolve |
| `ndf_reel_reuse.edl` | NDF | `0 / 198000` | `450 / 198450` | 450 | 15.015 s | `REEL_X` x2 | 1 个 wipe、1 个 key |
| `negative_offset.edl` | DF | `0 / -60` | `300 / 240` | 300 | 10.01 s | `NEG` x2 | 无 |
| **批次合计** | DF+NDF | 每个文件独立归零 | 每个文件独立归零 | **1950** | **65.065 s** | 3 组 | **3** |

每个文件的校验均为：

```text
firstCut.frame == 0
lastCut.frame  == totalFrames
lastCut.frame - firstCut.frame == totalFrames
```

DF 样例的首个绝对帧是 `23:59:50;00`，事件跨过午夜后，最后一个绝对时间码展开为
`24:00:30;00`，但 EDL 原标签仍是 `00:00:30;00`。JSON 同时保留两者：
`unwrappedRecordTimecode` 与 `recordTimecode`。

## 换算规则

所有时间码先变为**有符号绝对帧号**，再进入时间线：

- NDF：`帧号 = ((hh*3600 + mm*60 + ss) * nominalFPS) + ff`
- DF：`帧号 = 上面的标签帧号 - drop * (总分钟数 - 总分钟数/10)`
- 29.97 DF 的 `nominalFPS=30`、`drop=2`；因此 `00:01:00;02` 是有效标签
  `1800`，而 `00:01:00;00` 会被拒绝。
- `30000/1001` 的 NDF 与 DF 使用同一个播放帧率，但标签计数不同：
  `01:00:00:00`（NDF）= `108000` 帧，`01:00:00;00`（DF）= `107892` 帧。
- 事件源出点或记录出点小于入点时，按 24 小时帧数向前展开。记录入点在上一条
  出点之后若出现“大负数”，同样判定为午夜回绕；小于半天的回退保留为重叠或负偏移。
- 记录时间线以所有事件的最小记录入点为 `0`，因此后面出现更早的负记录点也不会被
  错误夹到零。

源到记录的偏移有两种口径，JSON 都给出：

- `sourceToRecordOffsetFrames`：原始标签帧号之差，可为负；
- `absoluteSourceToRecordOffsetFrames`：跨午夜展开后的绝对帧差。

## 逐帧映射

事件保存 `recordTimelineInFrame`、`recordTimelineOutFrame` 和源区间；任意时间线帧
通过下式解析：

```text
sourceFrame = sourceInAbsoluteFrame + (timelineFrame - recordTimelineInFrame)
```

`FrameAt` 会返回事件号、reel、绝对源帧和展开后的源时间码。因为映射以事件号为准，
同一个 reel 被多次使用不会被合并；发生转场重叠时，后进入的事件优先。

`artifacts/frames.tsv` 是全部 1950 个时间线帧的逐行映射（另有一行表头），可直接核对首尾与同卷重复。

## 导回与转场损失

`ExportEDL` 只保留硬切：

- `C` 硬切：完整保留。
- `D` dissolve：源/记录切点和帧数保留，但混合曲线、source/target handles 没有
  可承载的 effect 节点，导出为硬切。
- `W...` wipe：图案、方向、边缘柔化参数丢失，导出为硬切。
- `K`/`KB` key：合成参数和透明度丢失，导出为硬切。
- 其他 fade/unknown 转场同样降级为硬切。

导出 EDL 会为每个降级点写 `* DROPPED TRANSITION:` 注释；完整原因和动作在
`artifacts/roundtrip/roundtrip-report.json` 的 `lostTransitions` 中。导回后再次解析，
`CompareCutRoundtrip` 会逐事件比较 reel、源入出点、记录入出点、首尾切点和总帧数。
当前样例结果为 3 个文件全部 `ok=true`，共 3 个转场降级。

## 运行

```bash
# 文本摘要
go run ./cmd/edlint \
  testdata/df_midnight.edl \
  testdata/ndf_reel_reuse.edl \
  testdata/negative_offset.edl

# 完整 JSON + 导回 EDL + 往返报告
go run ./cmd/edlint -out artifacts/timeline.json -export artifacts/roundtrip \
  testdata/df_midnight.edl testdata/ndf_reel_reuse.edl testdata/negative_offset.edl

# 逐帧映射
go run ./cmd/edlint -frames -frames-out artifacts/frames.tsv \
  testdata/df_midnight.edl testdata/ndf_reel_reuse.edl testdata/negative_offset.edl

go test ./...
```

`-fps 29.97` 是默认值并规范化为 `30000/1001`；`-mode auto` 优先读取每行的分号
和每个文件的 `FCM` 头。文件内的 `FCM` 变化也会生效。导出混合模式文件时，工具会在
需要 NDF 冒号标签的事件前写作用域 `* FCM: NON-DROP FRAME` 注释，避免文件级 FCM
把 NDF 行误读为 DF。

## 交付物

- `artifacts/timeline.json`：reel、源/记录入出点、帧率、绝对帧、时间线帧、负偏移、
  同卷复用、逐事件校验和转场损失。
- `artifacts/frames.tsv`：1950 条逐帧映射。
- `artifacts/roundtrip/*.roundtrip.edl`：降级为硬切的导回 EDL。
- `artifacts/roundtrip/roundtrip-report.json`：逐文件往返结果与损失说明。
- `artifacts/summary.txt`：文本核对摘要。

## 限制

- 一个时间线内要求相同的播放帧率；混合 23.976/25/30 的播放速率不在本工具范围内。
- 跨午夜按 24 小时循环展开；超过 24 小时的连续记录需要每段至少命中下一次回绕。
- CMX 3600 没有通用的转场参数承载结构，因此无法做到转场效果的无损往返；本工具保证
  帧级切点和源映射不被转场降级改变。
