# ACEScg to Rec.709

仅使用 Go 标准库的浮点色彩转换包。输入为场景线性 ACEScg（AP1 原色、
ACES D60 白点），输出为 D65 Rec.709。正反两个方向都使用 `float64`。

## 处理链

```text
ACEScg RGB
  -> XYZ (D60)                 ColorSpace.RGBToXYZ
  -> XYZ (D65)                 BradfordAdaptation
  -> linear Rec.709 RGB        ColorSpace.XYZToRGB
  -> BT.709 encoded RGB        EncodeBT709 / DecodeBT709
  -> limited-range integer     Quantizer.Encode / Decode
```

每一层都可以单独调用和测试。`Pipeline` 只负责校验并组合这些层；
反向矩阵由完整正向矩阵直接求逆，避免“逐级求逆再相乘”的额外误差。

## 用法

```go
pipeline, err := aces709.NewDefaultPipeline()
if err != nil {
	// 颜色常量、矩阵方向或白点配置错误
	return err
}

frame := []aces709.RGB{
	{R: 0.18, G: 0.18, B: 0.18},
	{R: 8, G: 4, B: 2}, // 超出 SDR 显示范围的高光
}
encoded, err := pipeline.ConvertFrame(frame)
if err != nil {
	return err
}

restored, err := pipeline.InvertFrame(encoded)
if err != nil {
	return err
}

q, err := aces709.Rec709Quantizer(10)
if err != nil {
	return err
}
codes, err := pipeline.ACEScgToRec709Codes(frame[0], q)
if err != nil {
	return err
}
back, err := pipeline.Rec709CodesToACEScg(
	[3]uint16{codes[0].Code, codes[1].Code, codes[2].Code}, q)
```

`Quantized.Clip` 会标记量化前的显示范围裁切：`ClipLow` 或 `ClipHigh`。
浮点路径不裁切，因此宽色域负值和 HDR 高值仍可逆。

## 关键约定

- `Matrix3` 是行主序矩阵，使用列向量；`a.Mul(b)` 表示先应用 `b`。
- ACEScg：AP1 原色，白点 `(0.32168, 0.33767)`，即 ACES D60。
- Rec.709：白点 `(0.3127, 0.3290)`，即 D65；使用 Bradford 色适应。
- BT.709 使用连续常量 `alpha=1.09929682680944`、
  `beta=0.018053968510807`。`[0,1]` 内是标准 OETF；负数和大于 1 的值使用
  奇对称扩展以保持浮点可逆。
- Rec.709 量化支持 8–16 bit，默认 limited/studio range：
  8 bit 为 16–235，10 bit 为 64–940，12 bit 为 256–3760。
- `ValidateConfig`/`NewPipeline` 会显式拒绝白点错误和矩阵转置、求逆或
  串用，返回 `ErrWhitePoint`、`ErrMatrixOrder` 或 `ErrInvalidMatrix`。

## 验证

```sh
go test ./...
```

测试由程序生成 33 档灰阶（约 `1e-4` 到 `1e3`）、ACEScg 纯色/补色，以及多档
HDR 高光。当前端到端浮点往返的最大绝对误差约 `5.7e-13`（输入峰值 `1e3`），
归一化最大误差约 `8.6e-16`；10-bit 灰阶量化误差按半码阶断言。
