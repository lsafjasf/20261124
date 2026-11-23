# ediff — 剪辑工程合并前变更单

`ediff` 比较两份剪辑工程（JSON 交换格式），输出一份**可核查的最小修改列表与冲突项**，
用于合并前评审。仅依赖 Go 标准库，无第三方依赖。

## 构建与运行

```bash
go build ./cmd/ediff
go test ./...

./ediff -format text examples/base.json examples/target.json
./ediff -format json  -out report.json examples/base.json examples/target.json
```

退出码：`0` 无冲突；`1` 存在冲突（可作为流水线门禁）；`2` 用法或解析错误。

## 工程格式

工程由媒体库 `media` 与轨道树 `tracks` 组成；轨道通过 `children` 嵌套，片段挂在轨道的
`clips` 上。片段关键字段：

- `timelineIn`：时间线位置；`sourceIn`/`sourceOut`：素材入出点；`speed`：速度（缺省按 1.0）
- `transition`：转场（类型、位置、时长、数值参数）
- `effects[]`：效果，按 `id`（缺失时退化为 `type`）对齐后逐参数比较

## 最小修改列表

比对以 ID 为对齐键，按字段产出最小粒度变更，操作类型：

| Op | 含义 |
| --- | --- |
| `track.add` / `track.remove` / `track.modify` | 轨道新增、删除、字段修改 |
| `clip.add` / `clip.remove` / `clip.modify` | 片段新增、删除、字段级修改 |
| `clip.move` | 纯移动（见下） |
| `media.add` / `media.remove` | 素材库新增、删除 |

### 移动判定（防止“整段替换伪装成移动”）

同一 ID 的片段，只有在**身份字段完全不变**、仅时间线位置或所在轨道变化时才记为
`clip.move`。身份字段为：`mediaId`、`sourceIn`、`sourceOut`、`speed`。

- 仅 `timelineIn` 变化 → `clip.move`，携带一个 `timelineIn` 字段变更
- 仅跨轨道变化 → `clip.move`，记录 `fromTrack`/`toTrack`
- 任一身份字段、转场或效果同时变化 → 一律 `clip.modify`，并注明
  `identity fields changed; this is a content modification, not a move`

## 冲突项

| Kind | 触发条件 |
| --- | --- |
| `DUP_CLIP_ID` | 单份工程内片段 ID 重复 |
| `DUP_TRACK_ID` | 单份工程内轨道 ID 重复（含嵌套） |
| `NAME_MEDIA_CONFLICT` | 同名片段绑定到不同 `mediaId`（工程内或跨工程） |
| `MEDIA_CONTENT_CONFLICT` | 同素材 ID 的路径或校验和不一致 |
| `CLIP_UNKNOWN_MEDIA` | 片段引用了媒体库中不存在的素材 |
| `BAD_SOURCE_RANGE` | `sourceOut < sourceIn` |
| `NESTED_TRACK` | 同 ID 轨道的嵌套层级/路径发生变化 |

## 可核查性

- 报告记录两个输入文件的路径与 SHA-256 指纹
- 变更与冲突按确定性规则排序，重复运行结果稳定
- JSON 报告供机器消费，文本报告供人工评审

## 目录结构

```
cmd/ediff        CLI 入口
internal/project 工程模型、加载、嵌套展开、结构校验
internal/diff    变更引擎：轨道/片段/素材/转场/效果比对与冲突检测
internal/report  文本报告渲染
examples         覆盖移动、参数修改、重复 ID、同名异媒的示例工程
```
