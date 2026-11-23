# retime

剪辑变速后的字幕/标记重定位工具。从旧序列（素材源帧）位置反查新序列（时间线）
位置，分别处理**变速**、**冻结**和**负速度**，无法唯一映射的事件输出候选并
等待人工确认；撤销时凭快照原样恢复速度曲线和全部标记。纯 Go 标准库实现，
`go.mod` 无任何第三方依赖。

## 数据模型

输入是一个项目 JSON：

- `speedCurve[]`：时间线起源的速度曲线段，按时间线顺序首尾拼接。
  - `srcStart`：该段起始源帧；`hold`：占用的源帧数（冻结时为保持时长）。
  - `speed`：精确有符号速率，字符串形式的有理数，如 `"2"`、`"1/2"`、`"-1"`、`"0"`。
  - 段时长精确公式：非冻结 `hold / |speed|`；冻结（`speed == "0"`）时长即 `hold`。
- `events[]`：字幕或标记，锚定在旧序列源帧。点事件只有 `start`，区间事件
  另有 `end`。

所有帧位置内部用 `math/big.Rat` 精确计算，落帧时按四舍五入（半数远离零）
取整，避免浮点漂移。

## 三种速度分支

- **变速（speed > 0）**：源帧 `s` 落在段 `[lo, hi)` 时，新位置为
  `段时间线起点 + (s-lo)/speed`，唯一映射。
- **冻结（speed == 0）**：整段源帧坍缩到同一个保持区间，点事件无法唯一
  落位，输出保持区间起点、终点两个候选（`freeze-hold-start/end`）。
- **负速度（speed < 0）**：源帧反向播放，新位置为
  `段时间线起点 + (hi-s)/|speed|`。单点仍唯一；区间事件若因此发生
  时间反转（新终点 < 新起点），附带人工方向确认警告。

段与段按半开区间 `[lo, hi)` 划分边界（最后一段含右端点），保证边界帧
归属唯一，不会被相邻段重复命中。

## 无法唯一映射的事件

`apply` 结果分两组：

- `uniqueMapped`：已确定 `newStart`/`newEnd`，可直接落位。
- `pending`：含 `candidates[]`，每个候选带帧号和原因，等待人工确认。
  `confirm` 只接受候选内的帧号，拒绝任何候选外选择。

## 撤销保证

`apply` 在清单里保存 `originalCurve` 和 `originalEvents` 的深拷贝快照，
与工作数据完全分离。`undo` 仅凭快照重建项目，原速度曲线和全部标记逐字段
还原（有测试保证 `reflect.DeepEqual`）。

## 用法

```
go build -o retime .

retime apply   project.json [manifest.json]   # 重映射，清单含撤销快照
retime confirm manifest.json choices.json [out.json]  # 人工确认候选
retime undo    manifest.json [project.json]   # 恢复原曲线和全部事件
```

`examples/` 含一份覆盖全部三种分支的端到端示例；`go test ./...` 覆盖
时长计算、三种映射、冻结候选、反转警告、手动确认、撤销还原和非法输入。
