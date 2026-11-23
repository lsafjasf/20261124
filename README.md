# FCPXML 兼容审计与保守修复

纯 Go、仅标准库实现的 FCPXML 审计器。它解决的是一个失败模式：
转换器输出了结构合法、可解析的文件，但转场变成硬切、`timeMap` 变速被删、
媒体引用或帧位置漂移，最后仍然返回成功。

本工具不会把“删掉效果”当作兼容。只要源时间线中的轨道、效果、媒体引用或
时间位置没有被候选文件完整表达，就记录为 `loss`，CLI 返回非零。修复时优先
保留候选文件；候选有丢失时，回退到源时间线，并对输出重新解析、重新对比。

## 审计维度

- 轨道：按 `sequence` / `spine` / `lane` 建立轨道，分别核对主轨、连接轨和
  嵌套 spine，不用全局 clip 数量代替轨道比较。
- 效果：核对 `transition`、`timeMap` 变速、`conform-rate`、
  `filter-video` / `filter-audio`，并比较效果资源、参数、关键帧和
  `timeMap` 的精确速度比。
- 媒体引用：解析所有 `ref`、`format`、`src`，检查资源 ID 是否存在、
  `media-rep` 是否有效；本地文件另行标记 `present` / `missing` / `remote`。
- 帧位置：用 `format.frameDuration` 做精确有理数换算，不经过浮点。
  每个 loss 同时给出秒、精确帧和四舍五入帧。
- 最早丢失点：所有 loss 按源序列时间排序，输出最早的帧位置、类别、代码、
  轨道和 XML 路径。

## 无损与安全规则

- XML 树保留元素顺序、属性、混合文本、注释、处理指令和未知节点；
  `Document.Bytes` 不会丢弃未知 FCPXML 结构。
- 资源 ID 被整体重编号不等于语义丢失：条目按媒体资源身份匹配，效果按
  effect UID/资源身份匹配。
- 滤镜元素还在但参数被改、渐变时长被改、`timeMap` 速度被改，仍然算 loss。
- 缺失本地媒体文件是 `warning`，因为 FCPXML 可以以离线媒体状态导入；
  悬空 `ref`、非法时间、重复资源 ID 等结构错误是 `loss`。
- 修复输出必须再次通过：XML 解析、结构审计、以及相对源时间线的全量对比。
  任一验证失败则不写文件，只输出不可表达清单。

## CLI

```sh
go run ./cmd/fcpxml-audit audit \
  -source testdata/source.fcpxml \
  -candidate testdata/lossy.fcpxml \
  -report artifacts/audit-report.json

go run ./cmd/fcpxml-audit repair \
  -source testdata/source.fcpxml \
  -candidate testdata/lossy.fcpxml \
  -out artifacts/repaired.fcpxml \
  -report artifacts/audit-report.json
```

`-report -` 把完整 JSON 报告写到 stdout；`-quiet` 关闭 stderr 的一行摘要。

退出码：

- `0`：没有 loss，源可重新导入所需的语义完整。
- `2`：检测到 loss，或源本身存在结构错误。即使已经生成保守修复文件也返回
  `2`，避免再次出现“有丢失但成功”。
- `1`：参数、读取、解析或写文件错误。

`repair` 的三种策略：

- `candidate_preserved`：候选无 loss，保留候选文件字节。
- `source_fallback`：候选有 loss，但源结构有效；输出源 FCPXML，恢复全部转场、
  变速和滤镜。这是保守策略，候选中的其他改动不会保留。
- `blocked`：源本身不可表达；不写修复文件，`repair.nonExpressible` 给出阻断项。

## Go API

```go
import "fcpxmlcompat"

source, err := fcpxml.ParseDocumentFile("source.fcpxml")
if err != nil {
    return err
}
candidate, err := fcpxml.ParseDocumentFile("converted.fcpxml")
if err != nil {
    return err
}

report, repair, err := fcpxml.AuditAndRepair(source, candidate)
if err != nil {
    return err
}
if report.Status != "compatible" {
    // 不可把 report.Summary.ConversionSucceeded 当作 true。
}
if repair.Report.Status == "ready" {
    if err := fcpxml.SaveRepair(repair, "repaired.fcpxml"); err != nil {
        return err
    }
}
```

只需要静态审计时调用 `AuditDocuments(source, nil)`；只需要比较时可用
`CompareDocuments` 或 `CompareInventories`。

报告中的关键字段：

```json
{
  "status": "lossy",
  "summary": {
    "lossCount": 3,
    "earliestLossFrame": "96",
    "earliestLossSeconds": "3.840000000",
    "conversionSucceeded": false
  },
  "earliestLoss": {
    "code": "TRANSITION_FLATTENED",
    "position": {
      "seconds": "3.840000000",
      "frameExact": "96",
      "frameRounded": 96
    }
  },
  "repair": {
    "strategy": "source_fallback",
    "reimportable": true,
    "preservesEffects": true
  }
}
```

## 测试夹具

- `testdata/source.fcpxml`：25 fps、主轨和 lane 1、三个 media-rep 资源、
  一个 Cross Dissolve 转场、一个 2/5 倍 `timeMap`、一个带参数滤镜。
- `testdata/lossy.fcpxml`：结构合法，但转场、转场滤镜和变速被删除。
- 测试断言最早丢失点为转场 offset 第 96 帧；修复文件必须仍含 1 个转场、
  1 个 `timeMap` 和 4 个效果，不能以删除效果通过。

仓库内已生成：

- `artifacts/audit-report.json`：当前失败样本的完整审计报告，最早丢失点为
  第 96 帧，3 个 effect loss，`repair.strategy=source_fallback`。
- `artifacts/repaired.fcpxml`：通过重解析和源时间线覆盖验证的保守修复文件，
  与源 FCPXML 字节一致，保留全部转场、变速和滤镜。

```sh
go test ./...
go vet ./...
```

## 已知边界

- FCPXML 未知子树会原样保留，但语义比较聚焦于轨道、时间、媒体引用、
  transition、timeMap 和滤镜参数。多机位 `mc-clip` 的机位内部、`audition`
  的备选片段目前只按外层条目核对。
- 非线性 `timeMap` 曲线会按完整 time point/参数指纹比较，但不执行曲线采样。
- “可重新导入”指结构、引用和时间表达可被 FCPXML 导入器接受；本工具不调用
  Final Cut Pro，因此不验证实际渲染像素或第三方插件是否安装。
