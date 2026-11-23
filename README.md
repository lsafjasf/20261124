# Sun AU 解析

仅使用 Go 标准库解析 Sun/NeXT AU（`.snd`、`.au`）音频。固定头部按
big-endian 读取；解析器不会修补头部，也不会复制、填充或重写任何原始字节。

```go
f, err := au.Parse(data)
if err != nil {
	return err
}

switch f.Format.Kind {
case au.KindMuLaw, au.KindLinearPCM:
	samples, err := f.Samples()
	if err != nil {
		return err
	}
	_ = samples
case au.KindUnknown:
	// 头部有效，但编码不在本包支持范围内。f.Data 仍指向原始 payload。
}
```

## 头部与数据区间

解析的固定头部为：

| 字段 | 偏移 | 类型 |
| --- | ---: | --- |
| magic（`.snd`） | 0 | 4 bytes |
| data offset | 4 | uint32 BE |
| data size | 8 | uint32 BE |
| encoding | 12 | uint32 BE |
| sample rate | 16 | uint32 BE |
| channels | 20 | uint32 BE |

支持的编码：

- `1`：8-bit G.711 μ-law
- `2`、`3`、`4`、`5`：8/16/24/32-bit 有符号线性 PCM
- 其他值：`KindUnknown`，包括 AU 的 float 和 A-law 编码

`DataSize == 0xffffffff` 表示长度未知，此时使用偏移后的全部实际字节。
否则只把声明范围内、物理上存在且能组成完整交错帧的字节放入 `Data`。
`ExtraBytes` 从 `Data` 末尾开始，覆盖声明范围外的字节和不足一帧的尾部。
若偏移超过文件末尾，`OffsetPastEOF` 为真且不会发生切片越界；偏移恰好等于
文件末尾表示合法的空 payload。

`File` 中的计数含义：

- `DeclaredBytes` / `DeclaredFrames`：头部声明的字节数和完整帧数
- `AvailableBytes` / `AvailableFrames`：文件偏移后实际存在的字节数和完整帧数
- `Frames`：最终保留在 `Data` 中的完整帧数
- `DeclaredSampleCount` / `AvailableSampleCount` / `SampleCount`：
  按声道交错计算的样本值数量

声明长度大于实际字节时，`Truncated` 和 `SizeMismatch` 为真；声明长度小于
实际字节时，`SizeMismatch` 为真且多出的字节仍在 `ExtraBytes` 中。若末尾
不足一个完整帧，该帧不会进入 `Data`，并由 `IncompleteTail`/`PartialBytes`
标记。

## 样本解码

`Samples()` 返回 `[]int32`，按声道交错排列。μ-law 展开为 16-bit 有符号值；
线性 PCM 使用大端读取和符号扩展。解码只读 `Data`，不会改动输入。

## 验证

```sh
go test ./...
go vet ./...
```
