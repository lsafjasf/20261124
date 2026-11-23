# 20261124

## PQ 测试图（纯 Go 标准库）

线性帧 -> PQ(ST 2084) 编码 -> 按 1000/2000/4000 nit 三种显示峰值解码的测试图合成器。
只用 Go 标准库（`image/png`、`math`），无任何图像/色彩第三方库。

运行：

```sh
go run .        # 打印采样表 + 生成 artifacts/pq_chart.png
go test ./...   # 往返精度 / 单调性 / 中灰峰值切换稳定性
```

### 量化边界

- **全程 float64**：场景线性 nit、PQ 信号 [0,1]、解码后的绝对 nit。
- **唯一量化点**：`QuantizePQ`（pq.go），PQ 信号 `round(pq*1023)` -> 10-bit 码。
- **显示端 8-bit**：`DisplayMap` 仅用于输出，超过显示峰值的码值标 CLIP 并截到 255，不回灌码流。

### 中灰稳定性

PQ 是绝对曲线（固定 0–10000 nit），码值与显示峰值无关。中灰 36.54 nit
（BT.2408 图形白 203 nit 的 18%）编码为 code 420，在三种峰值下解码都是
36.4058 nit，由 `TestMidGrayStableAcrossPeaks` 锁定。
