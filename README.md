# PGS 影院预览器

纯 Go 的 PGS（Blu-ray `.sup`）任意时刻预览工具。它按 PTS 回放显示集，
应用调色板透明度、PGS 窗口裁剪、对象裁剪与叠加，并可把 1920×1080 图形
映射到 1280×720 后输出 PPM 和窗口/对象坐标 JSON。

## 功能

- PCS / WDS / PDS / ODS / END 段解析，支持分片 ODS。
- PDS 调色板透明度（`T=0` 全透明，`T=255` 不透明），HD 使用 BT.709，
  SD（高度 ≤576）使用 BT.601。
- 同一 palette ID 的增量更新；仅更新 PDS 中出现的条目。
- PGS RLE 解码，包括 6 位、14 位、带显式颜色索引和行结束转义。
- 显示集状态机：任意时间点只应用 PTS 不晚于该时刻的 PCS，调色板更新
  不丢失当前对象组合。
- 对象裁剪矩形、对象与窗口求交、窗口与画布求交。
- 多个对象按 PCS 顺序做 source-over 预乘 alpha 叠加。
- 保持宽高比的分辨率映射；缩小使用面积平均，放大使用双线性插值。
- P6 PPM 输出，可铺黑、白、纯色或棋盘背景以观察透明效果。
- JSON 同时报告源坐标、裁剪后坐标、输出坐标和可见输出坐标。

## 快速开始

```bash
go test ./...

go run ./cmd/pgspreview \
  -input testdata/scenario.sup \
  -at 0.5 \
  -width 1280 \
  -background checker \
  -out preview.ppm
```

窗口和对象坐标默认写到标准输出的 JSON。可指定文件：

```bash
go run ./cmd/pgspreview testdata/scenario.sup 1.5 preview.ppm \
  -width 1280 -background checker -json report.json
```

时间支持普通秒数、Go duration、`HH:MM:SS.mmm` 或 `Nt`（90 kHz ticks）：

```bash
-at 1.5
-at 1500ms
-at 00:00:01.500
-at 135000t
-pts 135000
```

`-at` 的小数秒会向下映射到 PTS；`-pts` / `Nt` 保留精确 tick，避免
nanosecond 往返误差。

## 可复现检查场景

`testdata/scenario.sup` 由 `cmd/mkfixture` 生成，画布为 1920×1080：

```bash
go run ./cmd/mkfixture -out testdata/scenario.sup
```

场景中有两个窗口、三个对象，并包含以下检查点：

- `0.5s`：palette entry 2 的透明度为 128。
- `1.5s`：第一次调色板更新，entry 2 变为不透明。
- `2.5s`：第二次调色板更新，entry 2 变为全透明。
- 窗口 0 为 `(600,150,300,300)`，映射到 1280 宽后是
  `(400,100,200,200)`。
- 窗口 1 原始范围超出右边界，画布裁剪后为
  `(1800,900,120,180)`；其输出可见范围是 `(1200,600,80,120)`。
- 对象 3 位于 `(1850,920)`、大小 100×100，越出画布右边，
  可见部分为 `(1850,920,70,100)`，1280 输出为
  `(1233,613,47,67)`。
- 对象 1 和对象 2 在窗口 0 内重叠，用于验证后绘制对象覆盖前对象。

每次调色板更新后，JSON 中的 `objects` 仍然有三个条目，证明组合对象被保留。

## 命令行

```text
pgspreview [flags] <input.sup> [time] [output.ppm]

-input string        .sup 文件
-at string           展示时间
-pts int             90 kHz PTS，优先于 -at
-width int           输出宽；0 表示源宽
-height int          输出高；0 表示按宽高比推导
-out string          PPM 路径，默认 preview.ppm
-json string         JSON 路径；- 为 stdout，空字符串关闭
-background string   black、white、checker 或 #RRGGBB
-checker-size int    棋盘格边长，默认 16
```

PPM 没有 alpha 通道。`-background checker` 用灰阶棋盘直接显示哪些像素
是半透明或透明的；默认黑色背景则把透明度合成为黑色。

## JSON 坐标含义

```json
{
  "windows": [{
    "id": 1,
    "source": {"x": 1800, "y": 900, "width": 300, "height": 300},
    "clipped": {"x": 1800, "y": 900, "width": 120, "height": 180},
    "output": {"x": 1200, "y": 600, "width": 200, "height": 200},
    "output_clipped": {"x": 1200, "y": 600, "width": 80, "height": 120}
  }]
}
```

- `source`：WDS 中的原始窗口。
- `clipped`：窗口与画布求交后的可见范围。
- `output`：原始窗口映射到输出分辨率后的范围。
- `output_clipped`：窗口可见范围映射到输出分辨率后的范围。
- `objects[].composition`：PCS 中的原始对象位置和生效尺寸。
- `objects[].source`：对象内部使用的位图区域（考虑 crop）。
- `objects[].clipped`：经过对象、窗口、画布三重裁剪后的源坐标。
- `objects[].output`：对象最终可见区域在输出图中的坐标。

## 库接口

```go
stream, err := pgs.OpenSUP("subtitle.sup")
frame, err := stream.Render(pgs.RenderOptions{
    At:     1500 * time.Millisecond,
    Width:  1280,
    Height: 0, // 从宽高比推导 720
})
```

按精确 PTS 渲染：

```go
frame, err := stream.RenderAtPTS(135000)
```

`frame.Image` 是预乘 RGBA 的 `*image.RGBA`；也可以使用
`pgs.WritePPM` / `pgs.WritePPMFunc` 输出。
