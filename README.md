# admcheck — ADM 渲染链路折叠诊断

版本检查通过、但影院里所有对象都挤在中央时，用本工具把 **ADM 对象位置、
各输出通道能量、预计映射** 放到同一条时间线上，逐级定位对象在哪个加工
阶段被折叠，而不是只看总响度。

## 用法

```sh
go run ./cmd/admcheck -layout 0+5+0 [-downmix] [-o report.md] <adm.xml|bw64.wav>
```

- 输入：ADM XML 文件，或含 `axml` 块的 BW64/RF64/WAVE 文件
- `-layout`：目标布局 `0+2+0` / `0+5+0` / `4+5+0`（默认 `0+5+0`）
- `-downmix`：追加一级 LoRo 立体声下混（S4）

## 加工阶段模型

- **S1 源位置**：ADM 原始 audioBlockFormat 位置
- **S2 坐标归一**：cartesian/polar 归一化；坐标系不匹配或缺 position 时
  按 (0°,0°) 处理并记录折叠事件
- **S3 布局声像**：向目标布局声像化；高度丢失、DirectSpeakers 标签无法
  映射、divergence/screenEdgeLock 等在此记录
- **S4 下混**（可选）：LoRo 折叠，LFE 丢弃

## 报告内容

1. 分阶段折叠定位表：每个对象在 S1–S4 的状态与**首个折叠阶段**
2. 统一时间线：每个时间段 × 对象的位置 (az, el)、各通道能量 (dB)、
   预计映射（主导声道及占比）
3. 不可表达对象清单与人工处理建议
4. 折叠事件明细

`testdata/collapse.xml` 是复现"全部挤在中央"症状的样例（坐标系不匹配、
纯高度对象、缺 position、DirectSpeakers 声道不在布局，各一例，外加一个
正常对照对象）。示例输出见 `artifacts/report.md`。
