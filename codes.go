package cea608

// 控制码（第一字节归一化为 0x14，已去校验位），取第二字节。
const (
	ccRCL = 0x20 // Resume Caption Loading：进入 pop-on
	ccRU2 = 0x25 // Roll-Up 2 行
	ccRU3 = 0x26 // Roll-Up 3 行
	ccRU4 = 0x27 // Roll-Up 4 行
	ccRDC = 0x29 // Resume Direct Captioning：进入 paint-on
	ccEDM = 0x2C // Erase Displayed Memory：清显示内存
	ccCR  = 0x2D // Carriage Return：roll-up 滚动
	ccENM = 0x2E // Erase Non-Displayed Memory：清后台内存
	ccEOC = 0x2F // End Of Caption：交换显示/后台内存
)

// 特殊字符：0x11（ch1）/0x19（ch2）+ 0x30..0x3F
var specialChars = [16]rune{
	'®', '°', '½', '¿', '™', '¢', '£', '♪',
	'à', ' ', 'è', 'â', 'ê', 'î', 'ô', 'û',
}

// 扩展字符：0x12/0x1A（西班牙/法语），0x13/0x1B（葡萄牙/德语）
var extendedChars = [2][16]rune{
	{'À', 'Â', 'Ç', 'È', 'Ê', 'Ë', 'ë', 'Î', 'Ï', 'ï', 'Ô', 'Ù', 'ù', 'Û', '«', '»'},
	{'Ã', 'ã', 'Í', 'Ì', 'ì', 'Ò', 'ò', 'Õ', 'õ', '{', '}', '\\', '^', '_', '|', '~'},
}

// pacRowIndent 解析 PAC（Preamble Address Code），返回 0 起始行号与缩进列。
// b1 已归一化到通道 1（0x10..0x17），b2 已去校验位。
func pacRowIndent(b1, b2 byte) (row, indent int, ok bool) {
	if b2 < 0x40 || b2 > 0x7F {
		return 0, 0, false
	}
	var base int
	switch b1 {
	case 0x11:
		base = 1
	case 0x12:
		base = 3
	case 0x15:
		base = 5
	case 0x16:
		base = 7
	case 0x17:
		base = 9
	case 0x10:
		base = 11
	case 0x13:
		base = 12
	case 0x14:
		base = 14
	default:
		return 0, 0, false
	}
	row = base
	// 0x10 只有第 11 行一档；其余 b2>=0x60 表示偶数行。
	if b1 != 0x10 && b2 >= 0x60 {
		row++
	}
	indent = int(b2&0x1E) >> 1 * 4
	if indent > 28 { // 0x50..0x5F / 0x70..0x7F 为样式 PAC，无缩进
		indent = 0
	}
	return row - 1, indent, true
}

// oddParity 检查字节是否为奇校验（608 每字节最高位为奇校验位）。
func oddParity(b byte) bool {
	b ^= b >> 4
	b ^= b >> 2
	b ^= b >> 1
	return b&1 == 1
}
