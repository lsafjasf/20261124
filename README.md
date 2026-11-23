# flacseek

用 Go 编写的原生 FLAC 索引检查与重建工具。它不依赖外部解码器，直接扫描
metadata block、帧头、帧边界和 CRC，用来发现 SEEKTABLE 与实际音频帧不一致的问题。

## 构建与用法

```sh
go build -o flacseek ./cmd/flacseek

# 检查，输出给人看的报告
./flacseek check album.flac

# 机器可读报告
./flacseek check -json album.flac

# 把重建了 SEEKTABLE 的副本写到新文件
./flacseek repair album.flac album.fixed.flac
```

`check` 的退出码：

- `0`：音频和 seek index 均正常。
- `2`：音频帧可完整校验，但索引缺失、重复、越界或指向错误。
- `3`：音频损坏或被截断；不能通过只重建索引安全修复。
- `1`：参数、I/O 或文件格式错误。

`repair` 也支持 `-json`。输入与输出必须是不同文件；输出通过临时文件写入并校验，
成功后才原子改名到目标路径。输入文件始终只读。

## 校验内容

- 解析 `fLaC` marker 和全部 metadata block，检查 STREAMINFO、SEEKTABLE
  的长度、位置和基本约束。
- 按 FLAC 帧头格式解析固定/可变 blocking strategy、UTF-8 风格帧号或采样点、
  block size、采样率、声道数和位深。
- 对每个帧头校验 CRC-8；按下一个有效帧边界校验整帧 CRC-16。
- 校验帧号/采样点连续性，以及总采样数与 STREAMINFO 的一致性。
- 校验固定块流中除末帧外的块大小一致；可变块大小按每帧实际值处理。
- 对 SEEKTABLE 的每个点校验：
  - 相对首个音频帧的偏移是否正好是帧头位置；
  - `sample_number` 是否等于该帧实际首采样点；
  - `frame_samples` 是否等于该帧实际 block size；
  - 是否重复、乱序、越界；
  - 最后一个非占位点是否仍指向有效帧。
- padding/application/comment/picture 等其它 metadata block 原样保留。

## 分类规则

- `index_state=ok`：索引存在且所有点与真实帧一致。
- `index_state=repairable`：音频 CRC 和采样结构正常，问题只在于索引缺失、
  重复点、错误偏移/采样点/块大小、无效末点或畸形 SEEKTABLE 长度。
- `index_state=unusable`：音频截断、帧 CRC 失败、帧头失败、流结构不一致，
  或 STREAMINFO 等非索引 metadata 已不可安全重写。

`repair` 只接受 `audio_state=ok` 的输入。重建时按实际帧生成 seek 点：
通常每帧一点；若超过 metadata 24 位长度允许的上限，则等距抽样且始终包含最后一帧。

## 保证与边界

- 重建只替换（或插入）SEEKTABLE，音频帧字节逐字节复制。
- 写入前后分别计算输入 SHA-256，若输入哈希变化则拒绝提交结果。
- 输出会重新执行完整扫描，确认 `audio=ok` 且 `index=ok` 后才替换目标文件。
- 截断帧、重复 seek 点、可变 block size 均有单元测试覆盖。
- 不解码 PCM，因此不验证 STREAMINFO 的 MD5；音频完整性由帧结构、CRC-8、
  CRC-16、采样连续性和总采样数共同判断。
- 只接受以 `fLaC` 开头、音频延伸到文件末尾的原生 FLAC；不支持 ID3 前缀或
  音频后的附加标签。

## 测试

```sh
go test ./...
go vet ./...
```
