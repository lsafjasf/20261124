package stl

import "unicode/utf8"

type rowInfo struct {
	Number     int
	Cells      int
	StartIndex int
	Bytes      []byte
}

func (v *validator) subtitleContent(rec *subtitleRecord) ([]byte, []int) {
	var out []byte
	var origins []int
	for _, idx := range rec.TextIndex {
		b := v.doc.Blocks[idx]
		content := visibleContent(b.Text[:], b.isLastBlock())
		out = append(out, content...)
		for range content {
			origins = append(origins, idx)
		}
	}
	return out, origins
}

func (v *validator) subtitleRows(rec *subtitleRecord) ([]rowInfo, []int) {
	content, origins := v.subtitleContent(rec)
	if len(content) == 0 {
		return nil, origins
	}
	rows := make([]rowInfo, 0, 4)
	rowStart := 0
	rowNumber := 1
	for i := 0; i < len(content); i++ {
		if content[i] != lineBreak {
			continue
		}
		row := content[rowStart:i]
		block := 0
		if len(rec.TextIndex) > 0 {
			block = rec.TextIndex[0]
			if rowStart < len(origins) {
				block = origins[rowStart]
			} else if len(origins) > 0 {
				block = origins[len(origins)-1]
			}
		}
		rows = append(rows, rowInfo{
			Number: rowNumber, Cells: countCells(row, v.cct),
			StartIndex: block, Bytes: append([]byte(nil), row...),
		})
		rowNumber++
		rowStart = i + 1
	}
	if rowStart < len(content) {
		row := content[rowStart:]
		block := rec.TextIndex[0]
		if rowStart < len(origins) {
			block = origins[rowStart]
		} else if len(origins) > 0 {
			block = origins[len(origins)-1]
		}
		rows = append(rows, rowInfo{
			Number: rowNumber, Cells: countCells(row, v.cct),
			StartIndex: block, Bytes: append([]byte(nil), row...),
		})
	}
	return rows, origins
}

func countCells(row []byte, cct string) int {
	if len(row) == 0 {
		return 0
	}
	if utf8.Valid(row) {
		multi := false
		for _, r := range string(row) {
			if r >= utf8.RuneSelf {
				multi = true
				break
			}
		}
		if multi {
			n := 0
			for _, r := range string(row) {
				switch {
				case r == lineBreak || r == unusedSpace:
				case r <= 0x1F:
				case r >= 0x80 && r <= 0x85:
				case r >= 0x86 && r <= 0x9F:
				default:
					n++
				}
			}
			return n
		}
	}
	n := 0
	for _, b := range row {
		switch {
		case b == lineBreak || b == unusedSpace:
		case b <= 0x1F:
		case b >= 0x80 && b <= 0x85:
		case b >= 0x86 && b <= 0x9F:
		case cct == "00" && b >= 0xC1 && b <= 0xCF:
		default:
			n++
		}
	}
	return n
}

func hasUnusedSpace(field []byte) bool {
	return indexByte(field, unusedSpace) >= 0
}

func lastByte(content []byte) (byte, bool) {
	if len(content) == 0 {
		return 0, false
	}
	return content[len(content)-1], true
}
