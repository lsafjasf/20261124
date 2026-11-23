# MP4 / H.264 色彩标签同步器

仅使用 Go 标准库。发布员把审查后的色彩标签同时写入 MP4 容器和 H.264
码流，并对每一项变更提交旧值、新值、字节/位偏移和写入后的回读值。

## 同步范围

| 位置 | 字段 |
| --- | --- |
| MP4 `colr`（`nclx`） | `colour_primaries`、`transfer_characteristics`、`matrix_coefficients`、`video_full_range_flag` |
| `avcC` 中的每个 SPS VUI | `video_signal_type_present_flag`、`video_full_range_flag`、`colour_description_present_flag`、上述三个 8-bit 色度字段 |

若 SPS 没有 VUI、没有 `video_signal_type`，或没有
`colour_description`，补丁会在合法的 RBSP 位置补字段并重算
`avcC` 与祖先盒长度。若容器没有 `nclx`，会新增一个。`mdat` 始终按原字节
拷贝；测试同时比较 SHA-256 和原始 `mdat` payload。

## 安全规则

- 保留码点、未知码点会被拒绝。
- PQ (`16`) / HLG (`18`) 只能配 BT.2020 (`9`) 或 unspecified (`2`) 原色。
- identity matrix (`0`) 必须使用 full range。
- `avcC` 没有 SPS、存在多个互相矛盾的 `nclx`、或格式无法安全重写时返回
  `ErrCannotAutoFix`，不会猜测或部分写入。
- 所有检查和字节生成都在内存中完成。落盘流程为：同目录临时文件 →
  `fsync` → 临时文件回读 → 原文件硬链接备份 → 原子 `rename` → 目录
  `fsync` → 目标回读。
- 替换后任一回读失败都会用备份原子恢复，并再次比较原文件 SHA-256。非法
  请求或临时写入失败时原文件不会被触碰。

## 用法

```sh
# 只读检查
go run ./cmd/tagfix inspect artifacts/samples/container-bitstream-mismatch.mp4

# 应用审查结果并输出 JSON 审计报告
go run ./cmd/tagfix apply \
  -primaries 9 -transfer 16 -matrix 9 \
  -report /tmp/tagfix-report.json \
  artifacts/samples/container-bitstream-mismatch.mp4

# 重建四组样例与综合报告
go run ./cmd/tagfix selftest \
  -dir artifacts/samples \
  -report artifacts/conflict-report.json
```

`apply` 出错时仍会先输出报告，再以非零状态退出。库接口为
`tagfix.Build` / `tagfix.BuildTrack`（内存内）和 `tagfix.ApplyFile`
（原子落盘）。

## 报告字段

`changes[]` 每项包含：

- `target`：`container.colr.*` 或 `bitstream.avcC.sps[N].vui.*`
- `old` / `new`：审查前后值
- `byte_offset` / `new_byte_offset`：输入/输出文件中的绝对字节偏移；
  `-1` 表示原字段不存在
- `bit_offset` / `new_bit_offset`：字节内从 MSB 起算的位偏移
- `readback` / `readback_ok`：从实际落盘文件重新解析得到的值
- `changed`：该字段是否需要变化

`media_payload[]` 报告每个 `mdat` 的新旧偏移、大小和 SHA-256。
`rollback` 标明是否必须/执行/验证了恢复；恢复后 `readback` 会重新填成
磁盘上的旧值，而不是计划中的新值。

## 四组冲突样例

综合结果见 `artifacts/conflict-report.json`。每组样例的输入和输出分别在
`artifacts/samples/<name>.original.mp4` 与
`artifacts/samples/<name>.mp4`。

| 样例 | 冲突 | 结果 |
| --- | --- | --- |
| `container-bitstream-mismatch` | `colr` 为 BT.709，SPS VUI 为 BT.2020 PQ | 自动修复 |
| `missing-sps-color-description` | SPS 有 signal type，但颜色描述缺失 | 插入字段并自动修复 |
| `avcc-without-sps` | 容器有标签，`avcC` 没有 SPS | 不可自动修复，原文件保留 |
| `pq-with-bt709-primaries` | 请求 PQ + BT.709 原色，组合非法 | 拒绝并整体回滚 |

当前结果是 2 组自动修复、2 组不可修复，且所有样例的媒体载荷均未变化。

## 验证

```sh
go test ./...
go vet ./...
```
