# EBU STL 校验器

这是一个只读的 EBU Tech 3264（`.stl`）字幕校验工具，仅使用 Go 标准库。它读取
1024 字节 GSI 头和后续 128 字节 TTI 块，交叉检查声明值与文件实际情况，输出
GSI/逐块错误及修复建议，不会修改、重写或另存原文件。

## 快速开始

```bash
go run . subtitle.stl
go run ./cmd/stlcheck subtitle.stl
go run ./cmd/stlcheck -json subtitle.stl
go run ./cmd/stlcheck -strict subtitle.stl other.stl
go test ./...
```

退出码：

- `0`：没有 error；使用 `-strict` 时也没有 warning。
- `1`：存在校验 error，或 `-strict` 下存在 warning。
- `2`：参数错误或文件读取失败。

`-json` 面对单个文件时输出一个对象，多文件时输出对象数组。

## 检查内容

- 时间码：TCI/TCO 的时、分、秒、帧范围，TCO 不早于 TCI，同组 TCI 顺序，以及
  GSI TCF 是否等于第一个非注释字幕的 TCI。帧号按 DFC 的 25/30 fps 校验。
- 磁带/磁盘格式：DFC 是否为 `STL25.01` 或 `STL30.01`，TND/DSN 是否有效。若
  DFC 声明 25 fps 但实际出现第 25-29 帧，会单独报告帧率声明冲突。
- 显示模式：DSC `0`/`1`/`2`、VP 范围、JC/CS/CF，以及 open/teletext 控制码是否
  与 DSC 一致。
- 块序号：SGN/SN 小端解析，EBN 顺序、重复、保留值、终端 `FFh` 和残缺块。
- 声明数量：GSI 的 TNB/TNS/TNG 是否与实际块数、字幕记录数和组数一致。
- 语言/编码：CPN、LC、CCT 格式；LC 与 CCT 的常见脚本组合；TTI 中的 UTF-8
  多字节文本与单字节 STL 字符表的冲突。
- 空字幕：按“有可见文本”单独报告，不与其他错误混为一谈。
- 重复块：逐字节相同的 TTI，以及同一 SGN/SN 内重复的 EBN。
- 超长文本：行显示单元数超过 MNC、行数超过 MNR，以及最后文本块没有以
  `8Fh` 填充结束。报告会给出拆分/换行建议，绝不会通过截断字符来“修复”。

TTI 本身没有语言字段；TTI 文本编码由 GSI 的 CCT 决定。因此语言检查覆盖 GSI
LC/CCT 一致性以及 TTI 实际字节是否符合声明的单字节字符表。

## 报告结构

文本模式会显示声明值、实际值和逐块问题。每个 TTI 问题包含：

- 零基 `tti_index` 与一基 `tti_number`
- 文件偏移、SGN、SN、EBN
- `category`、`severity`、字段名、说明和 `suggestion`

JSON 中 `block` 为空表示 GSI 级问题；否则 `block` 指向具体 TTI 块。典型分类：

| 分类 | 含义 |
| --- | --- |
| `subtitle_count` | TNB/TNS/TNG 与实际结构不一致 |
| `timecode` | 时间码范围、顺序或 TCF 不一致 |
| `format` | DFC、TND/DSN、帧率冲突 |
| `display_mode` | DSC、VP 或控制码与显示模式不一致 |
| `language` | CPN/LC/CCT 无效或不兼容 |
| `block_number` | SGN/SN/EBN 顺序或终端块问题 |
| `empty_subtitle` | 字幕记录无可见文本 |
| `duplicate_block` | 重复 TTI 或重复块号 |
| `overlong_text` | 行/行数超限，或缺失 `8Fh` 终止填充 |
| `text_encoding` | 保留控制码、UTF-8 字节或悬空组合音标 |

修复建议只是文本，不会写回文件。涉及文本时建议增加扩展块、换行或拆分字幕并保留
全部字符，而不是截断。

## 包接口

```go
doc, err := stl.ParseFile("subtitle.stl")
if err != nil {
    // I/O error
}
report := doc.Validate()
if report.HasErrors() {
    // inspect report.Issues
}
```

`stl.Parse` 对损坏文件也采用宽容解析，因此即使 GSI 或最后一块不完整，仍可通过
`Validate` 得到逐块报告。
