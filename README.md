# TrueHD 截断扫描器

只使用 Go 标准库的 raw TrueHD（`.thd`）诊断工具。它不会因为末尾少几个字节
就把整个节目判为不可用，而是保留截断点之前所有完整的 access unit，并在损坏
区域之后寻找可信的 TrueHD 边界继续扫描。

## 错误分类

- `header_error`：AU 长度不可用、major-sync 签名/校验和错误、未知采样率、
  子流数量非法、AU/子流头奇偶校验错误，或子流长度互相矛盾。
- `payload_error`：帧头可信且 AU 完整，但子流末尾的 parity/CRC8 检查失败。
  只丢弃当前 AU，下一帧的 framing 仍然可信。
- `eof_error`：输入实际字节数少于帧头声明的 AU 长度，或者 EOF 落在头部中间。
  这类问题只归因于截断，不覆盖此前已经完整的帧。

每个问题都带：

- `packet_index`：扫描到的逻辑包序号；
- `raw_start` / `raw_end`：原始输入字节区间，右端点为开区间；
- `declared_size`：AU 头声明的字节数（可得到时）；
- `recovered_at`：坏头之后重新同步到的字节位置；
- `reason`：具体校验失败原因。

`header_error` 的区间可能跨多个包，因为长度字段损坏后无法可靠知道中间漏掉了
多少 AU。`payload_error` 和通常的 `eof_error` 都精确覆盖一个 AU 或文件尾。

## 可播放范围

`playable_ranges` 是完整、连续、可播放的 AU 段。每个段同时给出：

- 原始字节区间 `raw_start..raw_end`（右开）；
- 帧索引、帧数和采样数；
- 以解码后播放时间计算的起点/终点秒数。

坏包两侧的帧不会被强行合并成一个连续范围；报告会保留两个独立段。全局的
`playable_bytes`、`playable_samples` 和 `playable_duration_seconds` 只统计
真正通过的完整帧。

`discarded_packets` 与 `issues` 内容对应，便于调用方直接读取被丢弃的原始区间。

## 使用

```sh
go run ./cmd/truehdscan film.thd
go run ./cmd/truehdscan -json film.thd
cat film.thd | go run ./cmd/truehdscan -json -
go run ./cmd/truehdscan -frames film.thd
```

文本输出示例：

```text
size: 7000 bytes
status: truncated (header=0 payload=0 eof=1)
playable: 235 frame(s), 6978 byte(s), 9400 sample(s), 0.195833 s in 1 range(s)
  range 0: bytes [0..6978) (0x0..0x1b42) frames=0..234 samples=9400 decoded=0.000000..0.195833 s
discarded: 22 byte(s) across 1 issue(s)
  eof_error     packet=235 bytes=[6978..7000) (0x1b42..0x1b58) declared=24: truncated access unit: ...
```

扫描本身成功时退出码为 `0`，即使发现可恢复损坏或 EOF 截断；只有参数或 I/O
错误返回非零。这样自动化流程可以读取报告，而无需把“部分可播”当成命令失败。

## Go API

```go
data, err := os.ReadFile("film.thd")
if err != nil {
	return err
}
report := truehd.Scan(data)
for _, segment := range report.PlayableRanges {
	log.Printf("play bytes [%d,%d)", segment.RawStart, segment.RawEnd)
}
for _, lost := range report.DiscardedPackets {
	log.Printf("%s at [%d,%d): %s",
		lost.Kind, lost.RawStart, lost.RawEnd, lost.Reason)
}
```

## 判定细节

1. 从 AU 头的 12-bit 长度字段取得完整包长（单位是 16-bit word，因此乘以 2）。
2. 若 `AU+4` 处是 `F8726FBA`，解析 28 字节或带扩展的 major-sync 头，并验证
   TrueHD 的 CRC16/字段组合校验和。
3. 验证 AU 头加所有子流头的半字节奇偶校验，以及子流 `end` 偏移是否单调、
   是否落在 AU 内。
4. 对带 `checkdata_present` 的子流，验证末尾两字节的 parity 和 CRC8。
5. 坏头后，先寻找下一个带有效 major-sync 校验和的 AU；同时允许利用仍可信的
   子流数量/采样参数，在普通（非 major-sync）AU 边界恢复。普通边界必须通过
   完整头校验、可用负载校验，并且下一帧边界也可信，避免把负载中的偶然字节
   当成新帧。
6. 如果声明长度越过 EOF，则报告 `eof_error` 并停止扫描；此前通过的帧全部
   保留。

## 限制

- 输入是 raw TrueHD elementary stream，不是 M2TS/MP4 容器；容器内的 PES
  边界需先由调用方拆出。
- 不做音频解码，只验证 framing 和流内校验。
- 如果某个流没有 `checkdata_present`，payload 没有可验证的 CRC，此时会接受
  结构完整的 AU，并在 `unchecked_payload_frames` 中计数。
- 当前接口一次性读入输入；超大文件应在上层切段或使用内存映射后调用 `Scan`。

## 验证

```sh
go test ./...
go vet ./...
```

测试使用带真实 ffmpeg TrueHD major-sync 头的合成流，覆盖完整流、尾部截断、
坏头、坏负载、三类错误并存、跨多个包失帧后回 major sync 重同步，以及完全
没有 major-sync 的输入。
