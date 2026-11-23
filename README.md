# Rec.2020 → Rec.709 可解释色域边界压缩

纯 Go 标准库实现（无第三方依赖）。素材是场景线性 Rec.2020 RGB，允许出现负值与大于 1
的通道；输出为可显示的 Rec.709 线性/伽马值与 PPM 证据。

## 运行

```bash
go test ./...        # 单元测试 + 端到端策略对照测试
go run . artifacts   # 生成全部证据到 artifacts/
```

## 处理管线（先负 RGB，再色相/亮度保持压缩）

1. **负 RGB 提升** `liftNegatives`（`compress.go`）
   - 所有通道加常数 `-min(R,G,B)`，把最小值抬到 0（沿消色轴去饱和）。
   - 再用公共增益 `目标亮度/提升后亮度` 恢复原始加权亮度，色相与通道比例不变。
2. **固定矩阵转换** `Rec2020To709Linear`（`color.go`）
   - Rec.2020 线性 RGB → D65 XYZ → Rec.709 线性 RGB，使用 ITU 原色精确矩阵。
3. **二次负处理**：矩阵残差若仍有负通道，在 Rec.709 亮度权重下重复步骤 1。
4. **等亮度边界搜索** `compressToBoundary`（`compress.go`）
   - 沿 `out = (Y,Y,Y) + α·(c-(Y,Y,Y))` 等亮度线向消色点移动，64 次二分搜索
     最大可行 α，使结果恰好落在 Rec.709 RGB 立方体某个界面（如 `B=1`）。
   - Y>1 时先归一化亮度；Y≤0 的不可复现色度锚定到黑场并保留正信号。

## 三种策略对照

| 策略 | 实现 | 特点 |
| --- | --- | --- |
| `channel_clip` | 矩阵后逐通道截断到 [0,1] | 最常见的“霓虹糊块”路径，独立截断破坏色相与亮度 |
| `boundary_search` | 负提升 + 等亮度二分边界 | 保持相对亮度、色相不翻转，结果落在色域界面 |
| `fixed_matrix` | 仅固定矩阵，不做压缩 | 记录全部越界，仅在写 PPM 时为可显示而截断 |

## 指标定义

- **最大色差**：CIEDE2000（`labdelta.go`，含 Sharma 参考用例测试），比较源 Lab 与复现 Lab。
- **色相翻转像素**：源与输出均非中性（色度 ≥ 0.5），且 Lab 色相角圆周差 > 90°。
- **亮度误差**：同时报告 CIE Y 绝对误差与相对误差；相对误差只统计 `|Y|≥0.2`
  的可复现像素，Y≤0 锚定像素在 CSV 中留空。

## 证据文件（`artifacts/`）

- `source_rec2020_preview.p6.ppm` / `.p3.ppm`：源素材（16-bit P6 与 ASCII P3）。
- `out_of_gamut_mask.p6.ppm`：越界像素掩码。
- 每策略一套：`<strategy>.p6.ppm`、`<strategy>.p3.ppm`、
  `<strategy>_hueflip_mask.p6.ppm`、`<strategy>_deltaE_heatmap.p6.ppm`、
  `<strategy>_pixels.csv`（逐像素色差/亮度/α/界面/负偏移）。
- `report.txt`：三策略汇总指标、翻转像素清单、霓虹样例逐步解释。
