// Package cea608 实现 CEA-608 隐藏字幕解码器。
//
// 支持 pop-on、paint-on、roll-up 三种显示模式，各由独立状态机驱动；
// 具备成对控制码去重、行滚动、清屏、坏字节跳过、双字节断裂续传
// 与中断恢复能力。仅依赖 Go 标准库。
package cea608

import "strings"

const (
	Rows = 15 // 屏幕行数
	Cols = 32 // 屏幕列数
)

// Mode 表示当前字幕模式。
type Mode int

const (
	ModeNone Mode = iota
	ModePopOn
	ModePaintOn
	ModeRollUp
)

func (m Mode) String() string {
	switch m {
	case ModePopOn:
		return "pop-on"
	case ModePaintOn:
		return "paint-on"
	case ModeRollUp:
		return "roll-up"
	default:
		return "none"
	}
}

// screen 是一块 15x32 的字符内存（显示内存或后台内存）。
type screen struct {
	cells [Rows][Cols]rune
}

func (s *screen) clear() {
	*s = screen{}
}

func (s *screen) put(r, c int, ch rune) {
	if r >= 0 && r < Rows && c >= 0 && c < Cols {
		s.cells[r][c] = ch
	}
}

// scrollUp 将 [top, bottom] 区间整体上移一行，底行清空。
func (s *screen) scrollUp(top, bottom int) {
	if top < 0 {
		top = 0
	}
	for r := top; r < bottom; r++ {
		s.cells[r] = s.cells[r+1]
	}
	s.cells[bottom] = [Cols]rune{}
}

func (s *screen) line(r int) string {
	row := make([]rune, Cols)
	for c, ch := range s.cells[r] {
		if ch == 0 {
			row[c] = ' '
		} else {
			row[c] = ch
		}
	}
	return strings.TrimRight(string(row), " ")
}

// Decoder 是 CEA-608 解码器。调用 Decode 持续喂入字幕字节流
// （已从视频中提取的 cc 字节对，顺序任意切分均可）。
type Decoder struct {
	displayed screen // 显示内存
	stored    screen // 后台（非显示）内存

	state    modeState // 当前模式状态机
	rollRows int       // roll-up 窗口行数（2/3/4）
	row, col int       // 光标
	baseRow  int       // roll-up 窗口底行

	pending  []byte // 跨 Decode 调用的残缺字节（最多 1 个有效）
	lastCtrl uint16 // 上一个控制码，用于成对去重；0 表示无

	// BadPairs 统计因校验失败被丢弃的字节对数量。
	BadPairs int
}

// New 返回处于初始状态的解码器。
func New() *Decoder {
	return &Decoder{state: noneState{}}
}

// Reset 复位全部状态，用于换台、流中断后强制恢复。
func (d *Decoder) Reset() {
	*d = Decoder{state: noneState{}}
}

// Mode 返回当前模式。
func (d *Decoder) Mode() Mode {
	return d.state.mode()
}

// Lines 返回当前显示内存 15 行内容（右侧空格已裁剪）。
func (d *Decoder) Lines() []string {
	out := make([]string, Rows)
	for r := 0; r < Rows; r++ {
		out[r] = d.displayed.line(r)
	}
	return out
}

// String 渲染当前显示内存，便于调试。
func (d *Decoder) String() string {
	return strings.Join(d.Lines(), "\n")
}

// Decode 喂入一段字幕字节。字节数可以不是 2 的倍数：
// 残缺的首/尾字节会缓存到下一次调用，实现双字节断裂续传。
func (d *Decoder) Decode(data []byte) {
	d.pending = append(d.pending, data...)
	for len(d.pending) >= 2 {
		b1, b2 := d.pending[0], d.pending[1]
		d.pending = d.pending[2:]
		d.handlePair(b1, b2)
	}
}

// handlePair 处理一个字节对，是解码的总入口。
func (d *Decoder) handlePair(b1, b2 byte) {
	// 坏字节：校验失败即丢弃整对，绝不中断后续字幕。
	if !oddParity(b1) || !oddParity(b2) {
		d.BadPairs++
		d.lastCtrl = 0
		return
	}
	b1 &= 0x7F
	b2 &= 0x7F

	if b1 == 0 && b2 == 0 {
		return // 空填充对，不影响控制码去重
	}

	// 可打印字符对（基本字符集）。
	if b1 >= 0x20 {
		d.putChar(rune(b1))
		if b2 >= 0x20 {
			d.putChar(rune(b2))
		}
		d.lastCtrl = 0
		return
	}

	// 归一化通道 2 控制首字节到通道 1。
	c1 := b1
	if c1 >= 0x18 {
		c1 -= 0x08
	}

	switch {
	case c1 == 0x14 && b2 >= 0x20 && b2 <= 0x2F:
		// 控制码：608 要求发送两遍，紧邻的重复对必须忽略。
		code := uint16(b1)<<8 | uint16(b2)
		if code == d.lastCtrl {
			return
		}
		d.lastCtrl = code
		d.handleControl(b2)
	case c1 == 0x11 && b2 >= 0x30 && b2 <= 0x3F:
		d.putChar(specialChars[b2-0x30])
		d.lastCtrl = 0
	case (c1 == 0x12 || c1 == 0x13) && b2 >= 0x30 && b2 <= 0x3F:
		// 扩展字符覆盖前一字符位置。
		if d.col > 0 {
			d.col--
		}
		d.putChar(extendedChars[c1-0x12][b2-0x30])
		d.lastCtrl = 0
	case c1 == 0x11 && b2 >= 0x20 && b2 <= 0x2F:
		d.putChar(' ') // 行中样式码，占一个空格
		d.lastCtrl = 0
	case c1 == 0x17 && b2 >= 0x21 && b2 <= 0x23:
		d.col += int(b2 - 0x20) // Tab Offset 1..3
		d.lastCtrl = 0
	default:
		if row, indent, ok := pacRowIndent(c1, b2); ok {
			d.lastCtrl = 0
			d.state.pac(d, row, indent)
		}
		// 其余未知组合：忽略，继续后续字幕。
	}
}

// handleControl 分发控制码。模式无关的清屏/切模在此处理，
// 模式相关的 CR/EOC 交给当前状态机。
func (d *Decoder) handleControl(cmd byte) {
	switch cmd {
	case ccRCL:
		d.enterMode(popOnState{})
	case ccRDC:
		d.enterMode(paintOnState{})
	case ccRU2:
		d.enterRollUp(2)
	case ccRU3:
		d.enterRollUp(3)
	case ccRU4:
		d.enterRollUp(4)
	case ccEDM:
		d.displayed.clear()
	case ccENM:
		d.stored.clear()
	case ccCR:
		d.state.carriageReturn(d)
	case ccEOC:
		d.state.endOfCaption(d)
	}
}

func (d *Decoder) enterMode(s modeState) {
	d.state = s
	d.row, d.col = 0, 0
}

func (d *Decoder) enterRollUp(rows int) {
	d.rollRows = rows
	d.enterMode(rollUpState{})
	d.baseRow = Rows - 1
	d.row = d.baseRow
}

// putChar 经由当前模式状态机写入目标内存并推进光标。
func (d *Decoder) putChar(ch rune) {
	d.state.putChar(d, ch)
}
