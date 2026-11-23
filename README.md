# sf2inspect

SoundFont 2 (`.sf2`) 跨表引用检查器。纯 Go 标准库实现，解析 `phdr`、`pbag`、
`pgen`、`inst`、`ibag`、`igen`、`shdr` 七张表，把每个 preset 展开到它实际
触发的样本，并定位三类错误：缺失引用、循环点重叠、非法根键。

## 构建

```sh
go build ./cmd/sf2inspect
```

## 用法

```sh
sf2inspect [--json] file.sf2
```

- 文本模式逐条打印 `preset -> instrument -> sample` 展开链、键/力度范围、
  根键、声明循环点与应用 generator 偏移后的有效循环点。
- `--json` 输出同样的结构化数据，便于二次处理。
- 退出码：`0` 无 error；`1` 存在 error 级诊断；`2` 用法或文件解析失败。

## 展开规则

- 按 `phdr[i].presetBagNdx` 到 `phdr[i+1].presetBagNdx` 切出 preset 的 bag，
  instrument 侧同理使用 `inst.instBagNdx` 与 `ibag`。
- 第一个不含 `instrument`(pgen) / `sampleID`(igen) 的 zone 视为 global zone，
  其 generator 与后续本地 zone 合并；同一 generator 以本地 zone 为准。
- preset 层 generator 叠加在 instrument 层之下；instrument 层同名 generator
  覆盖 preset 层。
- 循环点同时报告 shdr 声明值和应用 `startloopAddrsOffset/CoarseOffset`、
  `endloopAddrsOffset/CoarseOffset` 后的有效值。
- 未在 SF2.04 表内的 generator id 不做语义解释，原样保留 id 与有符号数值，
  输出中标记为 `unknownNN` 且 `known=false`。
- 兼容两套索引编号：经典文件（TimGM6mb、Fluid R3 等）使用 `instrument=41`、
  `sampleID=37`，SF2.04 文本另列 `49/53`；两套都会被识别为索引 generator。
- 旧字体常用 ≥900 的 sampleID（如 977/1000）表示 ROM 或未使用 zone，这类
  引用报 warning 而非 error，并在展开时跳过。

## 诊断分类

- `missing-reference`：`instrument`/`sampleID` 索引越界、zone 缺少索引
  generator、shdr 数据或链接样本越界。位置精确到 `phdr[i].pgen zone k`、
  `inst[i].igen zone k` 或 `shdr[i]`；ROM 哨兵降级为 warning。
- `overlapping-loop`：单样本循环反转或超出自身数据范围、应用 generator 后
  有效循环越界，以及两个样本在 `smpl` 地址空间中的循环区间相交。
- `illegal-root-key`：shdr `originalPitch` 或 `overridingRootKey`/`keynum`
  generator 落在 0..127 之外（255 表示未指定，合法）。

另有 `structure` 类用于 bag/gen 索引倒挂、末端 EOP/EOI/EOS 记录缺失等
表结构问题，不与上述三类混报。

## 测试

```sh
go test ./...
```

测试内置了一个最小 SF2 构造器，覆盖正常展开、global zone 合并、三类错误
定位、未知 generator 保留、循环偏移计算与 RIFF 损坏处理。设置
`SF2_OUT=/path` 运行 `TestWriteExamples` 可把样例文件写到磁盘供 CLI 验证。
