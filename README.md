# 20261124

## eac3sm — E-AC-3 节目切换状态机（纯 Go）

修复问题：播放器在 E-AC-3 切节目时沿用旧 channel mode（acmod），导致下混矩阵错误、对白（中置声道）丢失。

### 设计

从 syncframe header 到解码器重建的显式状态机（`machine.go`）：

```
Acquire → AwaitInit → Streaming ⇄ RecoverCRC
                    ↘ Streaming ⇄ DRCUpdate
```

- **AwaitInit**：检测到新 program（demuxer boundary hint 或核心配置变化）后，必须收集到完整起始状态（连续 2 帧 header 一致、CRC 有效）才提交配置并重建解码器；期间丢帧静音，绝不用旧 channel mode 解码新节目。
- **RecoverCRC**：CRC 错走独立恢复路径——用已提交配置隐藏（conceal）坏帧，连续错超过 3 帧则丢同步回 Acquire；恢复时若配置已变则按新节目处理。
- **采样率变化**：触发完整解码器重建（经 AwaitInit → init-complete）。
- **DRC profile 变化**：走轻量路径 DRCUpdate，只重配 DRC，不重建解码器、无音频中断。

每次状态迁移都记录为 `Transition`（序号、帧号、源/目标状态、原因、配置快照），经 `Machine.Transitions()` 读取。

### 运行

```sh
go test ./...                 # 覆盖切节目/CRC/采样率/DRC 四类场景
go run ./cmd/eac3sm           # 打印帧事件与状态迁移记录
```

示例迁移记录见 `artifacts/transition-log.txt`。
