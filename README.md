# 时间线三方合并

纯 Go 标准库实现。以同一祖先时间线为基准，合并两位剪辑师分别产生的修改。

## 合并规则

- 轨道、片段、标记、转场均以稳定 `id` 识别，不依赖数组下标或修改时间。
- 对象按属性递归合并；双方修改不同属性时会自动合入。
- 实体数组按 `id` 求并集，因此双方新增不同片段/标记/转场不会互相覆盖。
- 片段通过 `trackId` 表达跨轨移动；移动与另一侧的参数修改可以合并。
- 转场参数逐项合并，例如一侧改 `duration`、另一侧改 `easing`。
- 同一属性在两侧被改成不同值时产生 `Conflict`，并列保留 `ancestor`、`ours`、`theirs` 三值。
- 冲突属性在合并结果中保持祖先值作为占位，绝不按时间戳或“哪边更新”自动选边。

## 使用

```bash
go run ./cmd/timeline-merge \
  -ancestor testdata/cross-track/base.json \
  -ours testdata/cross-track/ours.json \
  -theirs testdata/cross-track/theirs.json
```

输出包含合并后的完整时间线；有冲突时附加如下并排报告：

```text
CONFLICT clips[id="clip-hero"].start
  ancestor : 5
  ours     : 8
  theirs   : 11
```

参数 `-json` 输出 `{"timeline": ..., "conflicts": [...]}`。退出码：`0` 无冲突，`1` 有冲突，`2` 输入或参数错误。

作为库调用：

```go
result, err := timeline.Merge(ancestor, ours, theirs)
if err != nil {
    // 输入 ID 校验或编解码错误
}
if !result.Clean() {
    timeline.FormatConflicts(os.Stdout, result.Conflicts)
}
// result.Timeline 已包含全部无冲突修改
```

## 样本与测试

`testdata/cross-track` 覆盖跨轨移动叠加另一侧参数修改，`testdata/transition-params` 覆盖同一转场的不同参数修改，`testdata/additions` 覆盖双方无冲突新增，`testdata/conflict` 覆盖同一属性冲突。

```bash
go test ./...
go vet ./...
```

合并器的边界约定：无 `id` 的普通数组按原子值处理；实体排序沿用祖先顺序，再依次追加 `ours`、`theirs` 的新增项；删除与另一侧修改同一实体按冲突处理。
