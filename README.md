# SDR/HDR 逐帧审片（纯 Go，无外部解码工具）

本项目用 Go 标准库实现一个可复现的审片流水线，比较同一时间线的 SDR 与 HDR
两个版本，按帧核对帧号、切点、字幕边界和音频样本位置，并把正常色调差异与漏帧、
错误元数据严格分开。不调用 ffmpeg/ffprobe 或任何第三方解码库。

## 为什么不能只看构图

两版画面构图相同只说明几何内容一致，不说明它们来自同一时间线。HDR 版可能在
保持画面结构的同时：

- 漏掉整帧（后续帧 PTS 仍然连续，逐索引对比会错位）；
- 帧号（Num）元数据被写错，但画面内容还在；
- 色彩元数据自相矛盾（BT.2020 原色却声明 BT.1886 传递函数）；
- 字幕整体漂移若干 tick；
- 音频在样本时间线上留下空洞。

因此本工具先做**结构对齐**，再在对齐结果上做各项检查。

## 容器格式 VX1

素材使用自定义极简容器（`internal/vx`），纯标准库解析：

- 魔数 `VX1:` + 版本号；
- JSON 头：分辨率、帧率、像素格式（`yuv444p8`）、色彩元数据、音频格式；
- `V` 视频块：声明帧号 `Num`、`PTS`、关键帧/切点标志 + 三平面 YUV；
- `A` 音频块：起始样本位置 + 交错 PCM16；
- `S` 字幕块：JSON，tick 粒度起止边界；
- `E` 结束块。

## 结构指纹与对齐

色调映射（BT.1886 gamma 对 SMPTE ST.2084 PQ、BT.709 对 BT.2020 矩阵）会重排
梯度幅度但不改变边缘几何。指纹（`internal/inspect/fingerprint4.go`）因此采用：

- 空间梯度的秩（rank）前 K 个边缘，避免幅度阈值随色调漂移；
- 相邻帧时间差分的秩前 K 个运动边缘，提供逐帧身份；
- 边缘匹配容忍 1 像素位移。

对齐（`internal/inspect/align4.go`）用单调动态规划（Needleman-Wunsch 形式），
漏帧表现为一侧的 unpaired 帧，后续配对自动平移，不会被逐索引比较掩盖。

## 检查项与分类

| 类别 | severity | 检查 |
|---|---|---|
| 正常色调差异 | `normal-tone` | 结构匹配对上的帧，仅亮度等级不同、边缘几何一致 |
| 漏帧 | `defect` | SDR 有帧而 HDR 无对应结构（`frame-dropped-in-hdr`） |
| 帧号/PTS | `metadata-error` / `defect` | 声明 Num 与内容时间线不符、PTS 回退或漂移 |
| 切点 | `defect` | SDR 切点 tick 在 HDR 缺失或移动 |
| 字幕边界 | `defect` | 逐条字幕 tick 级起止对比 |
| 音频 | `defect` | 包级样本位置空洞、逐样本首个分歧点、总样本数 |
| 色彩元数据 | `metadata-error` | 原色/传递函数组合不合法或两版完全相同 |

## 复现

要求 Go 1.22+，无第三方依赖：

```bash
go test ./...
go run ./cmd/fixture   # 生成 artifacts/sdr.vx 与 artifacts/hdr.vx
go run ./cmd/inspect   # 生成 report.json 与异常帧 PNG
```

夹具（`internal/fixture`）是确定性合成的共享时间线：160x90、24fps、96 帧、
48kHz 单声道 PCM，包含 4 个场景切点、4 条字幕和移动标记。HDR 版注入的缺陷：

1. 第 41 帧（1.7083 s）漏帧；
2. 第 58 帧声明 `Num=700`（内容仍是第 58 帧）；
3. 头部 `primaries=bt2020` 但 `transfer=bt1886`（错误元数据）；
4. 全部字幕边界 +2 tick；
5. 第 41 帧对应的 2000 个 PCM 样本缺失，样本时间线留下空洞。

## 本次审片结论

- 最早**时间线**异常：字幕边界漂移，`0.0000 s`（字幕 1：SDR `[0,23]`，
  HDR `[2,25]`）。
- 头部元数据异常（不占时间线位置，单列）：HDR 色彩元数据自相矛盾。
- 漏帧点：`1.7083 s`（SDR tick 41），HDR 仅 95 帧；其后 SDR n 与 HDR
  存储帧 n-1 对齐，切点 tick `[0,24,48,72]` 在两版均保留。
- 音频首个样本分歧：位置 `82000`（1.708333 s），HDR 在样本 `82000→84000`
  有 2000 样本空洞，与视频漏帧同点。
- 95 对结构匹配帧全部归为 `normal-tone`：边缘几何一致，仅亮度等级差
  （平均 mean luma 差约 -68，方向与 PQ 提亮一致），最大结构距离 0.329。

产物：

- `artifacts/report.json`：完整发现、逐流清单（帧号、PTS、切点、字幕、音频）。
- `artifacts/anomaly_sdr.png` / `artifacts/anomaly_hdr.png`：最早异常时间点两版帧图。
- `artifacts/anomaly_pair.png`：该时间点左右并排对照。
- `artifacts/anomaly-boundary_*.png`：字幕漂移边界内侧（tick 1）对照，SDR 有
  字幕条而 HDR 此时尚未进入字幕区间。
- `artifacts/drop_sdr.png` / `artifacts/drop_hdr.png` / `artifacts/drop_pair.png`：
  漏帧时刻对照，时间码分别为 `00:00:01:17` 与 `00:00:01:16`，可直接看到一帧错位。
