# chromasiting

4:2:0 色度采样位置（chroma siting）识别与边界偏移分析工具，Go 单模块实现。

## 三种 siting

| siting | 2x2 亮度块内偏移 (x, y) | 说明 |
|---|---|---|
| `SitingMPEG2` | (0.0, 0.5) | MPEG-2 风格：水平与左侧亮度同位，垂直居中 |
| `SitingJPEG` | (0.5, 0.5) | JPEG 风格：水平垂直均居中（ITU T.81） |
| `SitingCenter` | (0.5, 0.5) | H.264 VUI `chroma_sample_location_type=1` |

JPEG 与 Center 物理位置相同，仅来源标签不同；`SamePosition` 在冲突比较前做归一化。

## 容器字段识别与冲突判定

`Identify(ContainerFields)` 汇总以下字段的声明（`fields.go`）：

- H.264 VUI `chroma_sample_location_type`（0=left，1=center；2-5 超出本工具范围）
- Matroska `ChromaSitingHorz/Vert`
- AV1 `chroma_sample_position`
- 编解码器默认值（`mjpeg`→JPEG，`mpeg2video`→MPEG-2，`mpeg1video`→Center）

判定规则：

- 所有在场字段物理位置一致，判定为 `resolved`；
- 任意冲突（即使 3 比 1 的多数派）或出现不支持的值，判定为 `manual_review`，
  列出全部字段声明，绝不自动选择最常见值，只能人工判定；
- 全部缺失，判定为 `unspecified`。

## 样本与边界测量

三类合成样本（`samples.go`），均为左红右蓝的垂直交界：

- `RedBlueBoundary`：偶数尺寸，边界落在两列之间；
- `OddSize`：奇数宽/高（色度面为 ceil(w/2) x ceil(h/2)）；
- `HalfPixelOffset`：边界偏移半像素，穿过像素中心（该列为 50% 混合）。

`MeasureBoundary` 对每种 siting 执行 4:4:4 → 4:2:0 → 4:4:4，分别测量上采样前
（色度域，经 siting 偏移映射回亮度坐标）与上采样后的红蓝边界位置；
`MeasureMismatch` 模拟"下采样用真实 siting、上采样用错误 siting"的错位后果，
边界会产生约 ±0.5 亮度像素的偏移——这正是字段冲突必须人工判定的代价。

## 运行

```sh
go run ./cmd/chromasiting   # 打印 siting 表、三样本边界对比、冲突判定示例
go test ./...               # 单元测试
```
