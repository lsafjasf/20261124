# ac3norm

位级精确的 AC-3（ATSC A/52，bsid ≤ 8）`dialnorm` 批处理编辑器，纯 Go 标准库实现。

针对“旧批处理把 `dialnorm` 改错导致响度跳变”的问题，本工具只翻转允许字段
（当前仅 5 bit 的 `dialnorm`）的位，逐位定位、不移动任何其他位，并在写入前后用
SHA-256 对比 payload 哈希，用重算的两个 CRC 保证码流仍然合法。

## 命令

```
ac3norm preview -manifest jobs.json   # 只校验并打印变更预览，不写任何文件
ac3norm commit  -manifest jobs.json   # 重新校验后原子写入每个文件
ac3norm inspect -file stream.ac3      # 打印逐帧字段、位偏移和 payload 哈希
```

清单格式见 `examples/jobs.json`：

```json
{"jobs":[{"path":"a.ac3","dialnorm":27,"boundaries":[48]}]}
```

- `dialnorm`：取值 0–31，超出范围拒绝。
- `boundaries`：可选，广告/节目拼接点的帧下标（从 1 开始严格递增）。

退出码：`0` 成功，`1` 用法/IO 错误，`3` 整批中止。

## 安全保证

1. **先预览后写入**：`preview` 只读；`commit` 在独立阶段重新读盘、重新校验，
   预览与提交永远基于同一份解析与编辑结果。
2. **整批停止**：出现下列任一情况立即中止，已处理文件保持原样，临时文件清理：
   - 输入帧 CRC1/CRC2 校验失败（CRC 冲突）；
   - `boundaries` 声明的拼接点两侧出现字节完全相同的重复帧。
3. **原子写入**：同目录临时文件 + `fsync` + `rename(2)` 覆盖，不会留下半写文件。
4. **位级编辑掩码**：除 `dialnorm` 的 5 个位和两个 CRC 字外，任何字节变化都视为
   非法并中止；帧长度不变。
5. **哈希对比**：每帧与整文件均输出修改前后 SHA-256；同值（no-op）编辑哈希不变。

## CRC 模型

与 FFmpeg/liba52 的 `AV_CRC_16_ANSI`（多项式 `0x8005`、MSB-first、初值 0）一致，
采用“零残差”约定：

- 帧在 5/8 点 `q = 2 * floor(5 * W_even / 8)` 处分为两段（`W_even` 为偶数
  frmsizecod 对的 16-bit 字数，44.1 kHz 填充帧与其偶数兄弟共享同一分界）；
- CRC1 区为字节 `[2, q)`，含 `[2,4)` 处的 CRC1 字，合法帧残差为 0；
- CRC2 区为字节 `[q, n)`，含末尾 CRC2 字，合法帧残差为 0。

编辑后通过 GF(2) 逆矩阵（按区域长度缓存）求出能把残差驱动回 0 的存储字，
写回后再解析一遍确认两个残差都为 0。
