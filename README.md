# 20261124

PGS 字幕 epoch 重建器：排查字幕服务器收到 PGS 后的残留窗口问题。

- `pgs/` — .sup 段解析 + 按 palette、window、object、display、clear 到达
  顺序重建 epoch 状态；报告每段 PTS/DTS、每个 display set 与 epoch 的精确
  起止时间（90 kHz 时钟），并定位首个错误 segment。epoch 开始时旧调色板、
  窗口、对象全部丢弃，跨 epoch 复用调色板会被判为错误。
- `cmd/pgsdump` — 命令行查看器：`go run ./cmd/pgsdump pgs/testdata/clean_two_epochs.sup`
- `cmd/pgsgen` — 重新生成二进制夹具：`go run ./cmd/pgsgen`
- `pgs/testdata/` — 二进制夹具：`clean_two_epochs.sup`（正常两 epoch）、
  `cross_epoch_palette.sup`（跨 epoch 复用旧调色板）、`residual_window.sup`
  （缺少 clear 导致残留窗口）。

测试：`go test ./...`（仅 Go 标准库）。
