# 20261124

离线操作日志的安全拼接与重放库（纯 Go 标准库，无第三方依赖）。

## 解决的问题

两份断联期间产生的操作日志直接拼接时，删除之前产生的旧移动操作会让
已删除片段"复活"。本库在重放每条事件前强制执行三类规则：

- **依赖检查**：`move`/`rename`/`delete` 要求目标片段当前存在，且目标
  路径未被其他片段占用，否则按 `missing_dependency` / `path_conflict` 拒绝。
- **过期事件拒绝**：每条事件携带 `expected_version`，与片段当前版本
  不一致即判定为过期（`stale_event`），旧移动不会在删除后复活片段。
- **撤销屏障**：显式 `barrier` 事件和 `delete` 都会建立屏障，屏障之前
  的操作不可再撤销；撤销一次删除是唯一合法的跨屏障操作，且会同步
  拆除该删除建立的隐式屏障。

## 断线重连流程

`ApplyAfterValidation` 先在引擎副本上整流校验，任一事件被拒绝则
返回 `ValidationError` 且真实状态不变；全部通过后才正式重放，
日志修复后可安全重试。

```go
merged := offlinelog.MergeLogs(logA, logB) // 按 (seq, source) 确定性全序
report, err := engine.ApplyAfterValidation(merged)
if err != nil {
    var ve *offlinelog.ValidationError
    errors.As(err, &ve) // 查看 ve.Rejections 修复日志
}
```

日志以 JSON 数组落盘（`EncodeLog`/`DecodeLog`），空日志编码为 `[]`。

## 测试

`go test ./...` 覆盖：交错删除与移动（两个方向）、重复重命名
（事件 ID 去重 + 版本过期）、撤销屏障与撤销守卫、空日志、
重连先校验后执行、日志 JSON 往返及畸形输入。
