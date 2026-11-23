package cea608

// modeState 是显示模式的独立状态机接口。
// 每种模式自行决定字符写入哪块内存、PAC 如何定位、
// 以及 CR / EOC 的语义，互不共享覆盖逻辑。
type modeState interface {
	mode() Mode
	// putChar 写入一个字符并推进光标。
	putChar(d *Decoder, ch rune)
	// pac 处理定位码（行、缩进）。
	pac(d *Decoder, row, indent int)
	// carriageReturn 处理 CR（仅 roll-up 有实际行为）。
	carriageReturn(d *Decoder)
	// endOfCaption 处理 EOC（仅 pop-on 有实际行为）。
	endOfCaption(d *Decoder)
}

// writeAt 是各状态机共用的落字原语：写入、推进、越界丢弃。
func writeAt(d *Decoder, s *screen, ch rune) {
	if d.col < Cols {
		s.put(d.row, d.col, ch)
	}
	d.col++
}

// noneState 尚未收到任何模式控制码：丢弃字符，避免脏数据上屏。
type noneState struct{}

func (noneState) mode() Mode                  { return ModeNone }
func (noneState) putChar(d *Decoder, ch rune) {}
func (noneState) pac(d *Decoder, row, indent int) {
	d.row, d.col = row, indent
}
func (noneState) carriageReturn(d *Decoder) {}
func (noneState) endOfCaption(d *Decoder)   {}

// popOnState：pop-on 模式。字符写入后台内存，EOC 时整屏翻出。
type popOnState struct{}

func (popOnState) mode() Mode { return ModePopOn }

func (popOnState) putChar(d *Decoder, ch rune) {
	writeAt(d, &d.stored, ch)
}

func (popOnState) pac(d *Decoder, row, indent int) {
	d.row, d.col = row, indent
}

func (popOnState) carriageReturn(d *Decoder) {} // pop-on 忽略 CR

func (popOnState) endOfCaption(d *Decoder) {
	// 交换显示/后台内存：新字幕上屏，旧字幕转入后台待覆盖。
	d.displayed, d.stored = d.stored, d.displayed
	d.row, d.col = 0, 0
}

// paintOnState：paint-on 模式。字符直接写入显示内存，逐字上屏。
type paintOnState struct{}

func (paintOnState) mode() Mode { return ModePaintOn }

func (paintOnState) putChar(d *Decoder, ch rune) {
	writeAt(d, &d.displayed, ch)
}

func (paintOnState) pac(d *Decoder, row, indent int) {
	d.row, d.col = row, indent
}

func (paintOnState) carriageReturn(d *Decoder) {} // paint-on 忽略 CR

func (paintOnState) endOfCaption(d *Decoder) {
	// EOC 隐含切回 pop-on：交换内存并切换状态机。
	d.displayed, d.stored = d.stored, d.displayed
	d.enterMode(popOnState{})
}

// rollUpState：roll-up 模式。固定 2/3/4 行窗口，CR 时向上滚动。
type rollUpState struct{}

func (rollUpState) mode() Mode { return ModeRollUp }

func (rollUpState) putChar(d *Decoder, ch rune) {
	d.row = d.baseRow // roll-up 字符始终落在窗口底行
	writeAt(d, &d.displayed, ch)
}

func (rollUpState) pac(d *Decoder, row, indent int) {
	// PAC 改变窗口底行位置；窗口内容不搬运，新行从新位置开始。
	d.baseRow = row
	d.row, d.col = row, indent
}

func (rollUpState) carriageReturn(d *Decoder) {
	top := d.baseRow - d.rollRows + 1
	d.displayed.scrollUp(top, d.baseRow)
	d.row, d.col = d.baseRow, 0
}

func (rollUpState) endOfCaption(d *Decoder) {} // roll-up 忽略 EOC
