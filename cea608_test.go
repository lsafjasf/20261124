package cea608

import (
	"strings"
	"testing"
)

// p 为 7 位字节补上奇校验位。
func p(b byte) byte {
	b &= 0x7F
	if oddParity(b) {
		return b
	}
	return b | 0x80
}

// cc 构造一个带校验的字节对。
func cc(b1, b2 byte) []byte { return []byte{p(b1), p(b2)} }

// text 把字符串编码为字节对序列，奇长度以 0x00 补齐。
func text(s string) []byte {
	out := []byte{}
	for i := 0; i < len(s); i += 2 {
		b2 := byte(0)
		if i+1 < len(s) {
			b2 = s[i+1]
		}
		out = append(out, p(s[i]), p(b2))
	}
	return out
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

// 常用指令流。
var (
	rcl      = cc(0x14, 0x20) // pop-on
	rdc      = cc(0x14, 0x29) // paint-on
	ru2      = cc(0x14, 0x25) // roll-up 2 行
	ru3      = cc(0x14, 0x26) // roll-up 3 行
	cr       = cc(0x14, 0x2D)
	edm      = cc(0x14, 0x2C)
	enm      = cc(0x14, 0x2E)
	eoc      = cc(0x14, 0x2F)
	pacRow14 = cc(0x14, 0x40) // 第 14 行（1 起始）
	pacRow15 = cc(0x14, 0x60) // 第 15 行
)

func displayedLines(d *Decoder) []string {
	var out []string
	for i, l := range d.Lines() {
		if l != "" {
			out = append(out, l)
			_ = i
		}
	}
	return out
}

func TestPopOnFlipsOnEOC(t *testing.T) {
	d := New()
	d.Decode(cat(rcl, pacRow14, text("HELLO")))
	if got := strings.Join(displayedLines(d), ""); got != "" {
		t.Fatalf("EOC 前不应上屏，got %q", got)
	}
	d.Decode(eoc)
	if got := d.Lines()[13]; got != "HELLO" {
		t.Fatalf("EOC 后第 14 行应为 HELLO，got %q", got)
	}
	if d.Mode() != ModePopOn {
		t.Fatalf("模式应为 pop-on，got %v", d.Mode())
	}
}

func TestPopOnEraseNonDisplayed(t *testing.T) {
	d := New()
	d.Decode(cat(rcl, pacRow14, text("DRAFT"), enm, eoc))
	if got := strings.Join(displayedLines(d), ""); got != "" {
		t.Fatalf("ENM 后 EOC 不应翻出内容，got %q", got)
	}
}

func TestPaintOnImmediateAndErase(t *testing.T) {
	d := New()
	d.Decode(cat(rdc, pacRow15, text("LIVE")))
	if got := d.Lines()[14]; got != "LIVE" {
		t.Fatalf("paint-on 应逐字直接上屏，got %q", got)
	}
	d.Decode(edm)
	if got := strings.Join(displayedLines(d), ""); got != "" {
		t.Fatalf("EDM 应清空显示内存，got %q", got)
	}
}

// TestRollUpFastScroll 模拟快速滚屏：行到达速度远高于阅读速度，
// 窗口只保留最后两行。控制码按规范成对发送，重复对不得重复滚动。
func TestRollUpFastScroll(t *testing.T) {
	d := New()
	stream := cat(ru2, ru2, pacRow15)
	lines := []string{"ONE", "TWO", "THREE", "FOUR"}
	for i, line := range lines {
		stream = cat(stream, text(line))
		if i < len(lines)-1 {
			stream = cat(stream, cr, cr) // 成对 CR 只滚一次
		}
	}
	d.Decode(stream)
	got := d.Lines()
	if got[13] != "THREE" || got[14] != "FOUR" {
		t.Fatalf("快速滚屏后应剩最后两行，got row14=%q row15=%q", got[13], got[14])
	}
}

func TestRollUpThreeLineWindow(t *testing.T) {
	d := New()
	stream := cat(ru3, pacRow15)
	lines := []string{"A", "B", "C", "D"}
	for i, line := range lines {
		stream = cat(stream, text(line))
		if i < len(lines)-1 {
			stream = cat(stream, cr)
		}
	}
	d.Decode(stream)
	got := d.Lines()
	if got[12] != "B" || got[13] != "C" || got[14] != "D" {
		t.Fatalf("3 行窗口应为 B/C/D，got %q/%q/%q", got[12], got[13], got[14])
	}
}

// popOnStream 构造一条完整的 pop-on 字幕流。
func popOnStream() []byte {
	return cat(rcl, rcl, pacRow14, text("HELLO"), pacRow15, text("WORLD"), eoc, eoc)
}

// TestSplitPairs 双字节断裂：同一字节流按不同粒度切碎喂入，
// 包括逐字节（每个字节对都被拦腰截断），结果必须与整段喂入一致。
func TestSplitPairs(t *testing.T) {
	stream := popOnStream()

	want := New()
	want.Decode(stream)

	for _, size := range []int{1, 3, 5, 7} {
		got := New()
		for i := 0; i < len(stream); i += size {
			end := i + size
			if end > len(stream) {
				end = len(stream)
			}
			got.Decode(stream[i:end])
		}
		if strings.Join(got.Lines(), "\n") != strings.Join(want.Lines(), "\n") {
			t.Fatalf("按 %d 字节切碎后结果不一致:\ngot:\n%s\nwant:\n%s",
				size, got.String(), want.String())
		}
	}
	if got := want.Lines()[13]; got != "HELLO" {
		t.Fatalf("第 14 行应为 HELLO，got %q", got)
	}
	if got := want.Lines()[14]; got != "WORLD" {
		t.Fatalf("第 15 行应为 WORLD，got %q", got)
	}
}

// TestBadBytes 坏字节：校验失败的字节对被丢弃，后续字幕照常解码。
func TestBadBytes(t *testing.T) {
	d := New()
	bad := []byte{0x41, 0x42} // 偶校验，非法
	d.Decode(cat(rcl, pacRow14, text("AB"), bad, text("CD"), eoc))
	if d.BadPairs != 1 {
		t.Fatalf("应统计 1 个坏字节对，got %d", d.BadPairs)
	}
	if got := d.Lines()[13]; got != "ABCD" {
		t.Fatalf("坏字节后应继续解码，got %q", got)
	}
}

// TestBadControlPairResync 控制码损坏：坏 EOC 不生效，重发的好 EOC 正常翻出。
func TestBadControlPairResync(t *testing.T) {
	d := New()
	badEOC := []byte{p(0x14), 0x2E} // 第二字节为偶校验，非法
	d.Decode(cat(rcl, pacRow14, text("OK"), badEOC))
	if got := strings.Join(displayedLines(d), ""); got != "" {
		t.Fatalf("坏 EOC 不应翻出字幕，got %q", got)
	}
	d.Decode(eoc)
	if got := d.Lines()[13]; got != "OK" {
		t.Fatalf("重发 EOC 后应翻出 OK，got %q", got)
	}
}

// TestInterruptRecovery 中断恢复：上一段字幕没收到 EOC 就断了，
// 新一段字幕（先 ENM 清后台）应正常解码上屏，无需人工干预。
func TestInterruptRecovery(t *testing.T) {
	d := New()
	d.Decode(cat(rcl, pacRow14, text("STALE"))) // 中断：EOC 丢失
	d.Decode(cat(rcl, enm, pacRow15, text("FRESH"), eoc))
	lines := d.Lines()
	if lines[14] != "FRESH" {
		t.Fatalf("中断后新字幕应正常上屏，got %q", lines[14])
	}
	if strings.Join(displayedLines(d), "") != "FRESH" {
		t.Fatalf("ENM 应清掉中断残留，got %q", d.String())
	}
}

// TestReset 换台/硬中断后 Reset 回到初始状态。
func TestReset(t *testing.T) {
	d := New()
	d.Decode(cat(rdc, pacRow14, text("OLD"), []byte{p('X')})) // 末尾留半个字节对
	d.Reset()
	if d.Mode() != ModeNone || d.BadPairs != 0 {
		t.Fatalf("Reset 后应回到初始状态")
	}
	if got := strings.Join(displayedLines(d), ""); got != "" {
		t.Fatalf("Reset 后屏幕应为空，got %q", got)
	}
	d.Decode(cat(rdc, pacRow15, text("NEW")))
	if got := d.Lines()[14]; got != "NEW" {
		t.Fatalf("Reset 后应可正常解码，got %q", got)
	}
}

// TestDuplicateControlCodes 成对控制码去重：紧邻重复只执行一次。
func TestDuplicateControlCodes(t *testing.T) {
	d := New()
	d.Decode(cat(rdc, rdc, pacRow14, text("X"), edm, edm))
	if got := strings.Join(displayedLines(d), ""); got != "" {
		t.Fatalf("EDM 重复对应只清一次（此处结果同为空屏），got %q", got)
	}

	// 关键场景：重复 CR 不得滚动两次。
	r := New()
	r.Decode(cat(ru2, pacRow15, text("ONE"), cr, cr, text("TWO")))
	lines := r.Lines()
	if lines[13] != "ONE" || lines[14] != "TWO" {
		t.Fatalf("重复 CR 应只滚动一次，got row14=%q row15=%q", lines[13], lines[14])
	}
}

// TestChannelTwo 通道 2 控制码应归一化处理。
func TestChannelTwo(t *testing.T) {
	d := New()
	d.Decode(cat(cc(0x1C, 0x29), cc(0x1C, 0x60), text("CH2")))
	if got := d.Lines()[14]; got != "CH2" {
		t.Fatalf("通道 2 paint-on 应正常上屏，got %q", got)
	}
}
