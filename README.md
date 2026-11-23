# 20261124

## cea608

CEA-608 隐藏字幕解码器，仅依赖 Go 标准库。

- 三种模式各自独立的状态机：pop-on（EOC 翻屏）、paint-on（逐字上屏）、roll-up（2/3/4 行窗口滚动）
- 成对控制码去重、PAC 定位、EDM/ENM 清屏、特殊/扩展字符、双通道归一化
- 容错：坏校验字节对丢弃后继续解码、双字节断裂跨 `Decode` 续传、中断后自动恢复、`Reset` 硬复位

```go
d := cea608.New()
d.Decode(ccBytes) // 可任意切分喂入
fmt.Println(d.String())
```

测试：`go test ./...`
