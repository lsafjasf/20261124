# clipreplay

一个只使用 Go 标准库的剪辑编辑日志：编辑操作按 JSON Lines 追加保存，
可崩溃恢复、确定性重放，并在重放偏离时定位首个差异步骤。

## 覆盖的操作

- 片段创建、删除、移动、修剪
- 片段变速
- 标记添加、移动、删除
- 效果添加、删除、改类型
- 效果参数设置、删除

主要 API 在 `timeline` 包中：

```go
base := timeline.NewProject("project-1", "Demo")
editor, err := timeline.CreateEditor("project.jsonl", base)
if err != nil {
    log.Fatal(err)
}
defer editor.Close()

err = editor.CreateClip("evt-1", timeline.Clip{
    ID: "clip-1", Track: "v1", Start: 0,
    SourceIn: 0, SourceOut: 1000, Speed: 1,
})
err = editor.MoveClip("evt-2", "clip-1", "v2", 500)
err = editor.TrimClip("evt-3", "clip-1", 500, 100, 800)
err = editor.SetSpeed("evt-4", "clip-1", 1.5)
err = editor.AddMarker("evt-5", timeline.Marker{
    ID: "marker-1", Time: 500, Label: "intro",
})
err = editor.AddEffect("evt-6", "clip-1", timeline.Effect{
    ID: "fx-1", Kind: "color", Params: map[string]float64{"gain": 0.5},
})
err = editor.SetEffectParam("evt-7", "clip-1", "fx-1", "gain", 0.8)
```

`eventID` 是幂等键。重复提交同一个 ID 时，第二条事件返回
`timeline.ErrDuplicateEvent`，工程、历史和日志都不改变。

## 崩溃恢复与重放

日志第一行保存一次性的基线工程，之后每行是一个 `Record`。每条记录包含：

- 当前操作 `op`
- 只针对该操作的逆操作 `inverse`
- 应用后的规范化工程 SHA-256 `state_hash`
- undo/redo 记录的原始目标事件 `target`

启动时，`OpenLog` / `OpenEditor` 的行为是确定的：

- 末尾未换行的残缺记录会被截断，并返回 `Recovery.TruncatedBytes`
- 完整但没有结尾换行的最后一条记录会被保留，并返回
  `Recovery.RepairedNewline`
- 已换行的损坏记录视为日志损坏，返回错误而不是猜测
- 重复 ID 在重放时按“首条胜出”跳过

重放从基线开始逐条应用。任一步出现操作错误、逆操作不一致或
`state_hash` 不一致时立即停止，返回 `*timeline.Difference`，其中
`Step` 和 `EventID` 就是首个差异步骤。

```go
replayed, diff, err := log.ReplayAndCompare(editor.Project())
```

重放完成后还会使用 `DiffProjects` 逐字段比较，差异路径形如
`Clips[0].Start` 或 `Clips[0].Effects[0].Params["gain"]`。逐条哈希用于
定位“步骤”，最终比较用于定位“字段”。

## Undo / Redo

撤销栈保存的是 `{EventID, Op, Inverse}`，重做栈保存同一组小命令；
不会为每次编辑复制整个 `Project`。Undo/Redo 本身也作为记录写入日志，
所以重新打开日志后，撤销/重做栈会从记录重建。

新建编辑会清空重做栈；撤销/重做分别要求目标是最新的栈顶事件。

## 确定性

- 片段按 ID、效果按 ID、标记按 `(time, id)` 规范化排序
- 所有操作使用绝对值参数
- 浮点参数拒绝 NaN 和 Inf
- 工程哈希基于规范化 JSON，map 键由 `encoding/json` 稳定排序

运行测试：

```sh
go test ./...
```
