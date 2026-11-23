# SDR / HDR10 双母版管线

从同一组线性浮点帧同时生成 SDR 与 HDR10 两套 10-bit 母版。镜头号、帧号、时码（如有）和字幕矩形在两条路径中原样写入元数据；字幕不烧录、不裁切、不改变位置。

## 运行

```bash
go run ./cmd/master -demo -out dist
```

不带 `-demo` 且不提供 `-input` 时同样会生成演示源。处理自有素材：

```bash
go run ./cmd/master -input /path/to/manifest.json -out /path/to/delivery
```

验证：

```bash
go test ./...
go vet ./...
```

仅使用 Go 标准库，`go.mod` 没有第三方依赖。

## 输入约定

- 源帧为 PFM `PF`（RGB float32），线性 Rec.2020 原色、D65 白点。
- 浮点值以 100 nit 参考白归一化：`0.18` 表示 18% 中灰，`8.0` 表示 800 nit。
- 允许负值和大于 1 的高光；NaN/Inf 会被拒绝。
- 源清单格式见 `dist/source/manifest.json`。

```json
{
  "version": 1,
  "project": "project-name",
  "working_space": "rec2020-linear",
  "frames": [
    {
      "shot": "S001",
      "frame": 42,
      "file": "frames/S001_0042.pfm",
      "subtitle": {
        "x": 80, "y": 310, "width": 480, "height": 30,
        "anchor": "bottom-center"
      }
    }
  ]
}
```

`config` 可省略，默认值为 100 nit 参考白、100 nit SDR 峰值、1000 nit HDR 峰值、BT.1886 gamma 2.4、10-bit limited range `64..940` 和确定性 Bayer 抖动。

## 输出

```text
delivery/
├── delivery.json          # 两套母版联合清单
├── validation.json        # 中灰、高光层次、峰值和 legal-range 报告
├── sdr/
│   ├── manifest.json
│   └── S001/S001_0042.rgb10le
├── hdr10/
│   ├── manifest.json
│   └── S001/S001_0042.rgb10le
└── previews/              # 仅用于人工看片，不是交付母版
    └── S001/S001_0042_compare.png
```

`.rgb10le` 为无头、逐像素交错的 `R,G,B` little-endian `uint16`，低 10 位有效。尺寸、色彩描述、文件大小和 SHA-256 位于对应 JSON。每套清单中的路径相对该清单目录；`delivery.json` 中的路径相对交付根目录。

HDR10 清单同时写入 BT.2020/D65/PQ、1000 nit mastering display、实际内容 `max_cll_nits` 和 `max_fall_nits`。

## 处理链

两条路径从同一个 `Vec3` 场景样本一次读取后分开：

1. **曝光基准**：`scene * 100 nit`；18% 中灰始终对应 18 nit。
2. **SDR**：18 nit 以下保持线性，高于 18 nit 使用连续软肩压缩到 100 nit；Rec.2020→Rec.709；色域外颜色沿等亮方向向中性轴去饱和；BT.1886 编码；10-bit limited-range 量化。
3. **HDR10**：900 nit 以下完全保留，900–1000 nit 软肩；保持 Rec.2020；PQ（SMPTE ST 2084）编码；同一 10-bit limited-range 量化。
4. **合法化**：每个输出通道最终裁到 `[64,940]`，编码后再次逐码字检查。抖动幅度小于一个码级，不会越界。
5. **元数据**：镜头、帧号、时码、描述和字幕矩形直接复制到两套清单。

软肩后仍为单调递增。因此同一高光带在 HDR 中保留明显码级差，而 SDR 被压到接近白端；18% 中性区在两条路径反解后保持同一绝对亮度。

## 演示验收

演示包含三个镜头、每个两帧：

- `S001_NIGHT`：夜景、城市窗灯、100/200/400/800 nit 窗格。
- `S002_WINDOW`：室内与窗外四级高光窗格。
- `S003_SKIN`：暖肤色特写与中性灰参照。

当前样例结果：

- 6 帧全部通过 `64..940` legal-range 检查。
- 中灰最大跨版本偏差 `0.0016 nit`。
- 100/200/400/800 nit 高光在 HDR10 中为约 `509/571/636/701` 码，最小步进 62；SDR 为约 `818/907/937/940`，最小步进 3。
- 内容峰值 `MaxCLL=800 nit`，最大帧平均光 `MaxFALL≈50.1 nit`。

`validation.json` 记录逐帧中灰、肤色 ROI、每级高光反解 nit、码级步进和峰值。`previews/*_compare.png` 左侧为 SDR、右侧为 HDR10，使用相同绝对亮度审查曲线，因此可直接比较中灰与高光层次；预览 PNG 不承担母版色彩管理职责。

## 范围说明

此工具产出的是 10-bit RGB HDR10/SDR finishing master，不包含 HEVC 编码与 MXF/MP4 封装；标准库不提供这些编码器。色彩、传递函数、legal range、动态元数据值和可追溯校验和均已完成，可交给后续编码环节。
