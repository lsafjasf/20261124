# HDR10 MP4 静态元数据审计器

只读审计 MP4/MOV 中容器与视频码流两处的 HDR10 静态元数据，核对
MaxCLL、MaxFALL、主色坐标、白点和母版亮度范围。实现只使用 Go 标准库。

## 审计口径

容器侧读取视频 sample entry（`hvc1`、`hev1`、`avc1`、`avc3`）下的
`mdcv` 和 `clli` box。码流侧读取：

- HEVC 前缀/后缀 SEI：payload type 137（MDCV）和 144（CLLI）；
- AVC SEI：直接 137/144，以及 ITU-T T.35 的 HDR10 形式；
- `hvcC`/`avcC` 中的参数集 SEI，以及普通/分片 MP4 的样本数据。

MDCV 采用 SMPTE ST 2086 顺序：绿、蓝、红、白点。所有值同时保留原始整数
和换算后的物理值：

| 字段 | 原始编码 | 换算 |
| --- | --- | --- |
| 主色/白点 x、y | `uint16` | `raw * 0.00002` |
| 最大/最小母版亮度 | `uint32` | `raw * 0.0001 cd/m²` |
| MaxCLL、MaxFALL | `uint16` | 直接为 cd/m² |

审计会检查坐标是否在 `[0,1]`、母版最大亮度是否不超过 10000 cd/m²、
`min <= max`、MaxCLL/MaxFALL 是否不超过 10000 cd/m²，以及
`MaxFALL <= MaxCLL`。另外会对色度接近零、母版亮度只有零点几尼特等
典型缩放错误给出 `*_unit_suspected` 错误。

## 冲突优先级

**冲突字段以码流 SEI 为准，容器 `mdcv`/`clli` 只作索引副本。**

原因是解码端实际消费的是码流中的 HDR10 SEI；容器副本用于检索、封装和
快速探测。审计按字段执行以下规则：

1. 码流字段唯一：采用码流值，即使容器值不同；若码流值越界或疑似单位错误，
   同时报告对应错误。
2. 码流缺该字段、容器存在唯一值：回退到容器值，并报告缺字段。
3. 同一层（容器或码流）出现多组不同值：视为歧义错误，不擅自挑选。
4. 两层都缺：报告 `hdr10_metadata_missing` 错误。

因此 JSON 报告中的 `precedence` 固定为 `bitstream_over_container`，每个
冲突项也带有 `winner: "bitstream"` 和原因。

## 使用

```bash
go test ./...
go run ./cmd/hdr10-audit testdata/batch/*.mp4
go run ./cmd/hdr10-audit -json testdata/batch/*.mp4
```

退出码：

- `0`：没有 error 级问题；
- `1`：审计发现 error（冲突、缺码流字段、单位/范围错误等）；
- `2`：参数或文件解析失败。

默认完全只读。仅显式使用 `-write` 才会改文件：

```bash
# 用码流值覆盖已有容器 mdcv/clli，原地写回
go run ./cmd/hdr10-audit -write conflict.mp4

# 写到新文件
go run ./cmd/hdr10-audit -write -o fixed.mp4 conflict.mp4
```

写回约束：

- 只覆盖已存在的 `mdcv`/`clli` payload，不新增、不删除 box；
- 文件长度和所有 chunk/sample offset 不变；
- 先在目标目录写临时文件，校验 `mdat` SHA-256 和各 `mdat` 区间完全一致，
  再原子替换目标；
- 容器缺 box 时只提示，不会为了补字段而移动媒体数据。

## 已覆盖样本

`go run ./cmd/hdr10-fixtures -out testdata/batch` 可重复生成以下合成样片：

- `aligned.mp4`：两处一致；
- `conflict.mp4`：MaxCLL、MaxFALL、主色和亮度均冲突；
- `missing-fields.mp4`：容器缺 CLLI、码流缺 MDCV，按字段回退；
- `duplicate-sei.mp4`：码流出现两组冲突 MDCV；
- `duplicate-box.mp4`：容器重复 `mdcv`/`clli`；
- `unit-errors.mp4`：坐标越界、亮度越界、CLLI 单位错误；
- `fragmented.mp4`：分片 MP4（`moof/tfhd/trun`）。

本次工作区没有随附真实 MP4，因此审计报告针对上述可复现样片生成：
`artifacts/audit.txt` 和 `artifacts/audit.json`。
