package stl

import (
	"fmt"
)

func (v *validator) checkRecords() {
	exact := make(map[[TTIBlockSize]byte]int)
	for _, b := range v.doc.Blocks {
		if first, ok := exact[b.raw]; ok {
			v.addTTI(b, SeverityError, CategoryDuplicate, "block",
				fmt.Sprintf("block is byte-for-byte identical to TTI[%d]", first),
				"Remove the later duplicate and renumber the subtitle's extension blocks; update TNB/TNS/TNG. "+
					"Do not delete text that differs from the earlier block.")
			continue
		}
		exact[b.raw] = b.Index
	}
	for _, rec := range v.records {
		v.checkBlockNumbers(rec, exact)
		v.checkEmptySubtitle(rec)
		v.checkHeaderFields(rec)
		v.checkControlCodes(rec)
		v.checkText(rec)
	}
}

func (v *validator) checkBlockNumbers(rec *subtitleRecord, exact map[[TTIBlockSize]byte]int) {
	expected := 0
	finalSeen := false
	seenEBN := make(map[uint8]int)
	for _, idx := range rec.BlockIndex {
		b := v.doc.Blocks[idx]
		if b.EBN >= 0xF0 && b.EBN <= 0xFD {
			v.addTTI(b, SeverityError, CategoryBlockNo, "EBN",
				fmt.Sprintf("extension block number 0x%02X is reserved for future use", b.EBN),
				"Use EBN 00h-EFh for extensions, FEh for user data or FFh for the final text block.")
			continue
		}
		if b.isUserData() || !b.isTextBlock() {
			continue
		}
		first, duplicate := seenEBN[b.EBN]
		if duplicate {
			if !isEarlierDuplicate(exact, b.raw, b.Index) {
				v.addTTI(b, SeverityError, CategoryDuplicate, "EBN",
					fmt.Sprintf("block number 0x%02X repeats within SGN=%d SN=%d (first at TTI[%d])",
						b.EBN, b.SGN, b.SN, first),
					"Give each extension block a unique ascending number 00h-EFh and keep exactly one final FFh block.")
			}
		} else {
			seenEBN[b.EBN] = b.Index
		}
		if b.isLastBlock() {
			if finalSeen && !duplicate {
				v.addTTI(b, SeverityError, CategoryBlockNo, "EBN",
					"another EBN FFh block appears after the subtitle was already terminated",
					"A repeated SN after FFh starts a new subtitle; assign the next SN instead of repeating FFh.")
			}
			finalSeen = true
			continue
		}
		if finalSeen && !duplicate {
			v.addTTI(b, SeverityError, CategoryBlockNo, "EBN",
				fmt.Sprintf("text block 0x%02X follows the terminal FFh block", b.EBN),
				"Start a new subtitle number for further text, or move the FFh marker to the actual final block.")
		}
		if int(b.EBN) != expected && !duplicate {
			v.addTTI(b, SeverityError, CategoryBlockNo, "EBN",
				fmt.Sprintf("expected extension block 0x%02X but found 0x%02X", expected, b.EBN),
				"Renumber extension blocks sequentially from 00h; finish with one FFh block.")
		}
		expected = int(b.EBN) + 1
	}
	if len(rec.TextIndex) > 0 {
		last := v.doc.Blocks[rec.TextIndex[len(rec.TextIndex)-1]]
		if !last.isLastBlock() {
			v.addTTI(last, SeverityError, CategoryBlockNo, "EBN",
				fmt.Sprintf("subtitle SGN=%d SN=%d has no final FFh text block", rec.Key.SGN, rec.Key.SN),
				"Mark the true final block FFh; if more text remains, add extension blocks instead of dropping characters.")
		}
	}
}

func isEarlierDuplicate(exact map[[TTIBlockSize]byte]int, raw [TTIBlockSize]byte, index int) bool {
	first, ok := exact[raw]
	return ok && first < index
}

func (v *validator) checkEmptySubtitle(rec *subtitleRecord) {
	header := rec.Header
	if header < 0 {
		if len(rec.BlockIndex) == 0 {
			return
		}
		header = rec.BlockIndex[0]
	}
	if !v.recordHasText(rec) {
		v.addTTI(v.doc.Blocks[header], SeverityError, CategoryEmpty, "TF",
			fmt.Sprintf("subtitle SGN=%d SN=%d has no visible text", rec.Key.SGN, rec.Key.SN),
			"Add the missing subtitle text or remove all blocks for this SN, then renumber extensions and update TNB/TNS/TNG.")
		return
	}
	for _, idx := range rec.TextIndex {
		b := v.doc.Blocks[idx]
		content := visibleContent(b.Text[:], b.isLastBlock())
		if emptyTextField(content) {
			v.addTTI(b, SeverityWarning, CategoryEmptyBlock, "TF",
				"TTI block contains no text content",
				"Remove the empty block or place the intended text in it; preserve every character from adjacent blocks.")
		}
	}
}

func emptyTextField(content []byte) bool {
	if len(content) == 0 {
		return true
	}
	for _, b := range content {
		if b != ' ' && b != lineBreak && b != 0x00 {
			return false
		}
	}
	return true
}

func (v *validator) checkHeaderFields(rec *subtitleRecord) {
	if len(rec.BlockIndex) == 0 {
		return
	}
	header := rec.Header
	if header < 0 {
		header = rec.BlockIndex[0]
	}
	b := v.doc.Blocks[header]
	if b.CS > 3 {
		v.addTTI(b, SeverityError, CategoryGSI, "CS",
			fmt.Sprintf("cumulative status 0x%02X is outside 00h-03h", b.CS),
			"Use CS 00h-03h or 00h when the subtitle is not part of a cumulative set.")
	}
	if b.JC > 3 {
		v.addTTI(b, SeverityError, CategoryDisplay, "JC",
			fmt.Sprintf("justification code 0x%02X is outside 00h-03h", b.JC),
			"Use JC 00h (unchanged), 01h (left), 02h (centred) or 03h (right).")
	}
	if b.CF > 1 {
		v.addTTI(b, SeverityError, CategoryGSI, "CF",
			fmt.Sprintf("comment flag 0x%02X is not 00h or 01h", b.CF),
			"Use CF 00h for transmission text or 01h for comments.")
	}
	switch v.doc.GSI.DSC {
	case '1', '2':
		if b.VP < 1 || b.VP > 23 {
			v.addTTI(b, SeverityError, CategoryDisplay, "VP",
				fmt.Sprintf("vertical position %d is not a teletext row 1-23", b.VP),
				"Set VP to the first teletext row occupied by the subtitle (1-23).")
		}
	case '0':
		if v.report.Declared.MNR != nil && int(b.VP) > *v.report.Declared.MNR {
			v.addTTI(b, SeverityError, CategoryDisplay, "VP",
				fmt.Sprintf("vertical position %d exceeds MNR %d", b.VP, *v.report.Declared.MNR),
				"Set VP within the declared open-subtitle row limit.")
		}
	}
}

func (v *validator) checkControlCodes(rec *subtitleRecord) {
	dsc := v.doc.GSI.DSC
	if dsc != '0' && dsc != '1' && dsc != '2' {
		return
	}
	for _, idx := range rec.TextIndex {
		b := v.doc.Blocks[idx]
		content := visibleContent(b.Text[:], b.isLastBlock())
		reportedControl := false
		reportedReserved := false
		for _, c := range content {
			switch dsc {
			case '0':
				if c <= 0x1F && !reportedControl {
					v.addTTI(b, SeverityError, CategoryDisplay, "TF",
						fmt.Sprintf("teletext control code 0x%02X appears in an open subtitle", c),
						"Remove teletext controls from open subtitles; use only 80h-85h open controls where needed.")
					reportedControl = true
				}
			case '1', '2':
				if c >= 0x80 && c <= 0x85 && !reportedControl {
					v.addTTI(b, SeverityError, CategoryDisplay, "TF",
						fmt.Sprintf("open-subtitle control code 0x%02X appears in teletext", c),
						"Use teletext controls 00h-1Fh or remove the open-subtitle control.")
					reportedControl = true
				}
				if c == 0x1B && !reportedReserved {
					v.addTTI(b, SeverityWarning, CategoryText, "TF",
						"teletext control code 0x1B is reserved for future use",
						"Remove or replace the reserved control; keep the surrounding text characters.")
					reportedReserved = true
				}
			}
			if (c >= 0x86 && c <= 0x89) || (c >= 0x8B && c <= 0x8E) ||
				(c >= 0x90 && c <= 0x9F) {
				if !reportedReserved {
					v.addTTI(b, SeverityWarning, CategoryText, "TF",
						fmt.Sprintf("reserved control code 0x%02X appears in the text field", c),
						"Remove or replace the reserved code; do not discard adjacent text characters.")
					reportedReserved = true
				}
			}
		}
	}
}

func (v *validator) checkText(rec *subtitleRecord) {
	content, origins := v.subtitleContent(rec)
	if len(content) == 0 {
		return
	}
	header := rec.Header
	if header < 0 {
		header = origins[0]
	}
	if hasUTF8, count := validUTF8Text(content); hasUTF8 {
		v.addTTI(v.doc.Blocks[origins[0]], SeverityWarning, CategoryText, "TF",
			fmt.Sprintf("text contains %d UTF-8 multibyte character(s), but STL TTI text uses CCT %s", count, v.cct),
			"Convert the text from UTF-8 to the declared STL character table, or correct LC/CCT; do not byte-truncate multibyte characters.")
	}

	rows, _ := v.subtitleRows(rec)
	if mnc := v.report.Declared.MNC; mnc != nil {
		for _, row := range rows {
			if row.Cells > *mnc {
				v.addTTI(v.doc.Blocks[row.StartIndex], SeverityError, CategoryOverlong, "TF",
					fmt.Sprintf("row %d has %d display cells but MNC is %d", row.Number, row.Cells, *mnc),
					"Reflow the row or split this subtitle into more cues while preserving all characters; do not truncate the text.")
			}
		}
	}
	if mnr := v.report.Declared.MNR; mnr != nil && len(rows) > *mnr {
		v.addTTI(v.doc.Blocks[header], SeverityError, CategoryOverlong, "TF",
			fmt.Sprintf("subtitle uses %d rows but MNR is %d", len(rows), *mnr),
			"Move complete rows into additional cues without dropping characters.")
	}
	if len(rec.TextIndex) == 0 {
		return
	}
	last := v.doc.Blocks[rec.TextIndex[len(rec.TextIndex)-1]]
	if last.Text[TextFieldSize-1] != unusedSpace {
		detail := "last text block does not end with 0x8F unused space"
		if !hasUnusedSpace(last.Text[:]) {
			detail = "last text block has no 0x8F unused space"
		}
		v.addTTI(last, SeverityError, CategoryOverlong, "TF",
			detail+"; a full 112-byte field may already have cut text off",
			"Continuation text must go into additional EBN 00h-EFh blocks with a final FFh block ending in 8Fh. Preserve every character; never shorten text just to make it fit.")
	}
	if v.cct == "00" {
		if b, ok := lastByte(content); ok && b >= 0xC1 && b <= 0xCF {
			v.addTTI(last, SeverityWarning, CategoryText, "TF",
				fmt.Sprintf("text ends with ISO 6937 accent code 0x%02X but no base character", b),
				"Continue the accented character in the next extension block, or restore the missing base character.")
		}
	}
}

func (v *validator) checkFrameRateAndTimecodes() {
	maxFrame := 0
	maxFrameIndex := -1
	prevTCI := make(map[uint8]Timecode)
	var first *subtitleRecord
	for _, rec := range v.records {
		if len(rec.BlockIndex) == 0 {
			continue
		}
		header := rec.Header
		if header < 0 {
			header = rec.BlockIndex[0]
		}
		b := v.doc.Blocks[header]
		if msg := timecodeRangeError(b.TCI, v.fps, true); msg != "" {
			v.addTTI(b, SeverityError, CategoryTimecode, "TCI", msg,
				"Use hours 0-23, minutes/seconds 0-59 and frames below the DFC frame rate.")
		}
		if msg := timecodeRangeError(b.TCO, v.fps, true); msg != "" {
			v.addTTI(b, SeverityError, CategoryTimecode, "TCO", msg,
				"Use hours 0-23, minutes/seconds 0-59 and frames below the DFC frame rate.")
		}
		if b.TCO.Before(b.TCI) {
			v.addTTI(b, SeverityError, CategoryTimecode, "TCO",
				fmt.Sprintf("TCO %s is before TCI %s", b.TCO, b.TCI),
				"Correct TCO so it is equal to or later than TCI, or split a crossing-midnight cue explicitly.")
		}
		if int(b.TCI.Frames) > maxFrame {
			maxFrame = int(b.TCI.Frames)
			maxFrameIndex = b.Index
		}
		if int(b.TCO.Frames) > maxFrame {
			maxFrame = int(b.TCO.Frames)
			maxFrameIndex = b.Index
		}
		if b.CF != 0 || len(rec.TextIndex) == 0 {
			continue
		}
		if first == nil {
			first = rec
		}
		if previous, ok := prevTCI[b.SGN]; ok && b.TCI.Before(previous) {
			v.addTTI(b, SeverityError, CategoryTimecode, "TCI",
				fmt.Sprintf("TCI %s is before %s of the previous subtitle in group %d",
					b.TCI, previous, b.SGN),
				"Keep TCI ascending within a subtitle group, or move the cue to the correct group.")
		}
		prevTCI[b.SGN] = b.TCI
	}

	if tcp, err := parseTimecodeString(v.doc.GSI.TCP); err == nil &&
		int(tcp.Frames) > maxFrame {
		maxFrame = int(tcp.Frames)
		maxFrameIndex = -1
	}
	if tcf, err := parseTimecodeString(v.doc.GSI.TCF); err == nil &&
		int(tcf.Frames) > maxFrame {
		maxFrame = int(tcf.Frames)
		maxFrameIndex = -1
	}
	if v.fps == 25 && maxFrame >= 25 {
		related := []int(nil)
		if maxFrameIndex >= 0 {
			related = []int{maxFrameIndex}
		}
		v.report.Issues = append(v.report.Issues, Issue{
			Category: CategoryFormat, Severity: SeverityError, Field: "DFC",
			Message: fmt.Sprintf("DFC declares 25 fps, but a time code contains frame %d, which requires 30 fps", maxFrame),
			Suggestion: "If the source is 29.97/30 fps, set DFC to STL30.01; otherwise correct the frame fields. " +
				"Do not renumber only some time codes.",
			RelatedIndexes: related,
		})
	}
	if v.doc.GSI.TCS == '1' && first != nil && v.doc.GSI.TCF != "" {
		tcf, err := parseTimecodeString(v.doc.GSI.TCF)
		if err == nil {
			h := first.Header
			if h < 0 {
				h = first.BlockIndex[0]
			}
			tci := v.doc.Blocks[h].TCI
			if tcf != tci {
				v.addGSI(SeverityError, CategoryTimecode, "TCF",
					fmt.Sprintf("TCF %s is not the TCI %s of the first non-comment subtitle", tcf, tci),
					fmt.Sprintf("Set TCF to %s, the TCI of the first transmitted subtitle.", tci.HHMMSSFF()))
			}
		}
	}
}

func (v *validator) checkSubtitleNumbers() {
	previous := make(map[uint8]uint16)
	for _, rec := range v.records {
		if len(rec.BlockIndex) == 0 {
			continue
		}
		if last, ok := previous[rec.Key.SGN]; ok {
			want := last + 1
			if rec.Key.SN != want {
				header := rec.Header
				if header < 0 {
					header = rec.BlockIndex[0]
				}
				v.addTTI(v.doc.Blocks[header], SeverityError, CategoryBlockNo, "SN",
					fmt.Sprintf("SN %d does not follow %d in subtitle group %d", rec.Key.SN, last, rec.Key.SGN),
					fmt.Sprintf("Renumber SN consecutively within the group (next is %d) without deleting or truncating text.", want))
			}
		}
		previous[rec.Key.SGN] = rec.Key.SN
	}
}
