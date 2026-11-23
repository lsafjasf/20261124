# colorshift — sRGB → ACEScg → Rec.709 转换器

纯 Go 标准库实现，无任何第三方依赖。

## 管线

1. **输入解释**：sRGB 码值经 sRGB EOTF（IEC 61966-2-1）解码为线性光。
   负值按镜像扩展处理，不裁剪。
2. **中间域**：线性 sRGB(D65) 经 Bradford CAT 适配到 ACES 白点(D60)，
   再变换到 ACEScg(AP1) 场景线性工作域。曝光补偿（`stops`）在此域以
   `2^stops` 倍增——它是创意控制，不能用来掩盖传递函数错误。
3. **输出**：ACEScg 逆变换回 Rec.709 线性光，经 BT.709 OETF 编码，
   量化为 8/16-bit PNG。仅最终显示量化时可裁剪到 [0,1]，
   中间域的合法负值全程保留。

## 使用

```sh
go run ./cmd/colorshift [-depth 8|16] [-exposure stops] in.png out.png
```

## 已修复的历史缺陷（逐段定位）

| 段 | 缺陷 | 现象 | 修复 |
| --- | --- | --- | --- |
| 输入解释 | 跳过 sRGB EOTF，码值当线性光 | 二次 Gamma，暗部抬升发灰 | 输入必须经 sRGB EOTF 解码 |
| ACEScg 中间域 | 中间值裁剪到 [0,1] | 合法负值被裁，宽色域色相偏移 | 中间域全程保留负值 |
| 8/16-bit 路径 | 16-bit 只取高字节 | 暗部渐变带状量化 | 16-bit 按 65535 归一化，全程 float64 |

前后误差对比见 `docs/ERROR_TABLE.md`（由 `go run ./cmd/errreport` 自动生成）。

## 回归样例

`testdata/` 内含 sRGB 源图、修复前（buggy）渲染、修复后 8/16-bit
期望输出。源图或管线变更后重新生成：

```sh
go run ./cmd/regen
```

## 测试

```sh
go test ./...
```
