# vobsubtool

VobSub (.idx/.sub) 字幕检查工具。仅依赖 Go 标准库，不需要字体或播放器。

## 用法

```
go build -o vobsubtool .
./vobsubtool [-o 输出目录] [-lang N] [-ascii] movie.idx
```

- 读取 `.idx` 的时间表、调色板、语言流（`id: xx, index: N`）。
- 按 `filepos` 在 `.sub` 中定位 pack/PES（private_stream_1）包，校验子流号
  `0x20+index`，解析 SPU 控制序列（调色板/透明度/坐标/RLE 偏移）并解码 RLE 位图。
- 每条字幕输出：时间戳、语言流、`.sub` 中的字节范围、位图尺寸，以及 PNG 预览
  （棋盘底衬显示透明区）；`-ascii` 同时在终端打印字符画预览。
- 错误落到具体条目：截断的 `.sub`（filepos 越界、PES 长度超出文件尾）、
  多语言流错配（idx 指向了其他语言的包）、控制序列/RLE 损坏等，
  都会以 `entry #N [时间戳 lang filepos]` 格式报出，且不影响其他条目。
  有条目失败时退出码为 1。

`testdata/movie.{idx,sub}` 是合成样例，覆盖正常、多语言同刻、流错配与截断场景。
