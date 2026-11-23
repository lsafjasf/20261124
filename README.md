# j2kindex

`j2kindex` 解析 JPEG 2000 raw codestream（`.j2k` / `.j2c`）的结构，不调用图像解码器，也不读取像素。它为网络切片、截断和拼接后的码流建立可核对的 packet 索引。

## 能力

- 解析 `SOC`、`SIZ`、`COD`、`COC`、`SOT`、`SOD`、`EOC` 及常见 marker。
- 解析 `SIZ`/`COD` 中的 tile、component、quality layer、resolution、precinct 和 code-block 参数。
- 根据 LRCP、RLCP、RPCL、PCRL、CPRL progression 重建每个 packet 的 `tile`、`layer`、`resolution`、`component`、`precinct` 索引。
- 解析 packet header 的 inclusion/zero-bitplane tag-tree、coding-pass 和 code-block 长度字段，从而在不解码像素的情况下确定 packet header 与 body 的边界。
- 支持多 tile、多 tile-part、SOP/EPH、自定义 precinct 和跨 tile-part 的 packet 状态。
- 所有结构问题都带半开字节区间 `[start,end)`。

## 使用

```bash
go run ./cmd/j2kindex image.j2k
go run ./cmd/j2kindex -pretty=false image.j2k > index.json
cat image.j2k | go run ./cmd/j2kindex -
go run ./cmd/j2kindex -strict image.j2k
```

`-strict` 在结果包含 `error` 时退出，状态码为 3。

## JSON 结果

顶层字段：

- `soc`、`eoc`：起止 marker 的字节区间。
- `siz`、`cod`：图像/瓦片几何与默认编码参数。
- `markers`：发现的 marker 列表，`scope` 为 `main` 或 `tile`。
- `tiles`：按 tile index 排列；每个 tile 包含有效 coding 参数、component/resolution 几何和 tile-part。
- `issues`：按发现顺序记录的结构问题。

每个 packet：

```json
{
  "index": 3,
  "layer": 1,
  "resolution": 2,
  "component": 0,
  "precinct": 0,
  "header": {"start": 120, "end": 124},
  "data": {"start": 124, "end": 151},
  "empty": false,
  "codeBlockBytes": 27
}
```

`index` 在单个 tile 内从 0 开始递增；`layer` 从 0 开始，是 quality layer 索引；`resolution` 从低分辨率 0 开始。`header` 不含 SOP 和 packet body；若存在 SOP/EPH，会另列在 `sop`/`eph`。

## 诊断

常用 issue code：

- `truncated_marker`、`missing_eoc`：marker 或定界符被截断。
- `duplicate_tile_part`：同一 tile 的同一 `TPsot` 重复；`related` 指向首次出现的 SOT。
- `tile_part_out_of_order` / `missing_tile_part`：`TPsot` 乱序或缺失。
- `length_overflow`：marker 长度、`Psot` 或 packet/code-block body 长度越过可用字节。
- `truncated_packet_header`：packet header 的 bitstream 不完整。
- `missing_eph`、`invalid_sop`：期望的 packet marker 不匹配。
- `missing_packets`：tile 的 progression 尚未完成，但数据已结束。
- `packet_indexes_skipped`：码流使用 `POC` 或 `PPM`/`PPT`，因此只做 marker/tile-part 结构分析，不输出不可靠的 packet 索引。

例如：

```json
{
  "code": "duplicate_tile_part",
  "severity": "error",
  "message": "tile 0 has duplicate TPsot 1",
  "range": {"start": 912, "end": 926},
  "related": [{"start": 455, "end": 469}]
}
```

## Go API

```go
data, _ := os.ReadFile("image.j2k")
result := j2k.Analyze(data)
for _, tile := range result.Tiles {
    for _, packet := range tile.Packets {
        fmt.Println(packet.Layer, packet.Resolution, packet.Header, packet.Data)
    }
}
```

`Parse` 是与 `Analyze` 对应的返回 `(*Codestream, error)` 形式；结构损坏记录在 `Issues` 中而不是丢弃已解析结果。

## 边界

不进行反量化、逆变换或码块解码。当前对 `POC`、`PPM`/`PPT` 打包 packet header 以及 HTJ2K code-block style 给出显式告警/错误，不会静默假设其语义。

## 测试

```bash
go test ./...
```
