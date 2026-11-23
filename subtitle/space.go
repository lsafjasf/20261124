package subtitle

import "unicode"

func (r ReadableSpace) canonical() ReadableSpace {
	if r.WidthCells < 1 {
		r.WidthCells = 1
	}
	if r.HeightLines < 1 {
		r.HeightLines = 1
	}
	return r
}

// runeWidth 按单元格宽度估算：东亚宽/全角字符计 2 格，其余计 1 格。
func runeWidth(r rune) int {
	if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
		return 2
	}
	if unicode.Is(unicode.Mn, r) || r == '\u200b' {
		return 0
	}
	return 1
}

// CellWidth 返回文本在等宽单元格模型下的宽度。
func CellWidth(text string) int {
	width := 0
	for _, r := range text {
		width += runeWidth(r)
	}
	return width
}

// WrapLine 在不超过 maxCells 个单元格的前提下按词折行，
// 返回折出的行数；无法在一行内容纳的长词会被强制截断。
func WrapLine(text string, maxCells int) []string {
	if maxCells < 1 {
		maxCells = 1
	}
	var lines []string
	var current string
	currentWidth := 0
	flush := func() {
		if current != "" {
			lines = append(lines, current)
		}
		current = ""
		currentWidth = 0
	}
	for _, word := range splitWords(text) {
		wordWidth := CellWidth(word)
		switch {
		case wordWidth > maxCells:
			flush()
			lines = append(lines, splitHard(word, maxCells)...)
		case current == "":
			current = word
			currentWidth = wordWidth
		case currentWidth+1+wordWidth <= maxCells:
			current += " " + word
			currentWidth += 1 + wordWidth
		default:
			flush()
			current = word
			currentWidth = wordWidth
		}
	}
	flush()
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func splitWords(text string) []string {
	var words []string
	start := -1
	for i, r := range text {
		if unicode.IsSpace(r) {
			if start >= 0 {
				words = append(words, text[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		words = append(words, text[start:])
	}
	return words
}

func splitHard(word string, maxCells int) []string {
	var lines []string
	var current []rune
	currentWidth := 0
	for _, r := range word {
		w := runeWidth(r)
		if currentWidth+w > maxCells {
			lines = append(lines, string(current))
			current = nil
			currentWidth = 0
		}
		current = append(current, r)
		currentWidth += w
	}
	if len(current) > 0 {
		lines = append(lines, string(current))
	}
	return lines
}

// CueFit 描述一条提示在给定可读空间下的适配结果。
type CueFit struct {
	Lines      int  // 按可读宽度折行后的总行数
	WidthCells int  // 折行后最宽一行的单元格数
	Fits       bool // 折行后宽度与行数是否都在可读空间内
	Overflow   int  // 行数超出可读高度的部分（不超为 0）
}

// FitCue 计算提示在当前可读空间下的适配情况。
func FitCue(cue Cue, space ReadableSpace) CueFit {
	space = space.canonical()
	lines := 0
	worstWidth := 0
	for _, line := range cue.Lines {
		wrapped := WrapLine(line, space.WidthCells)
		lines += len(wrapped)
		for _, wrappedLine := range wrapped {
			if width := CellWidth(wrappedLine); width > worstWidth {
				worstWidth = width
			}
		}
	}
	if cue.WidthCells > worstWidth {
		worstWidth = cue.WidthCells
	}
	overflow := lines - space.HeightLines
	if overflow < 0 {
		overflow = 0
	}
	fits := overflow == 0 && worstWidth <= space.WidthCells
	return CueFit{
		Lines:      lines,
		WidthCells: worstWidth,
		Fits:       fits,
		Overflow:   overflow,
	}
}
