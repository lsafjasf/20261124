package stl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type subtitleKey struct {
	SGN uint8
	SN  uint16
}

type subtitleRecord struct {
	Key        subtitleKey
	BlockIndex []int
	TextIndex  []int
	Header     int
}

type validator struct {
	doc      *Document
	report   Report
	records  []*subtitleRecord
	byKey    map[subtitleKey]*subtitleRecord
	fps      int
	language *uint8
	cct      string
}

// Validate checks declarations against the actual TTI blocks. It returns a
// report even for structurally damaged files.
func (d *Document) Validate() Report {
	if d == nil {
		d = &Document{}
	}
	v := &validator{
		doc:   d,
		byKey: make(map[subtitleKey]*subtitleRecord),
	}
	v.report = Report{
		SizeBytes:     d.Size,
		GSIComplete:   d.GSIComplete,
		TrailingBytes: d.Trailing,
		Issues:        []Issue{},
	}
	v.report.GSI = v.gsiInfo()
	v.fps = declaredFrameRate(d.GSI.DFC)
	v.cct = d.GSI.CCT
	v.collectRecords()
	v.fillActual()
	v.checkStructure()
	v.checkGSI()
	v.checkCounts()
	v.checkLanguage()
	v.checkRecords()
	v.checkSubtitleNumbers()
	v.checkFrameRateAndTimecodes()
	sortIssues(v.report.Issues)
	for _, issue := range v.report.Issues {
		if issue.Severity == SeverityError {
			v.report.Summary.Errors++
		} else {
			v.report.Summary.Warnings++
		}
	}
	return v.report
}

func (v *validator) gsiInfo() GSIInfo {
	g := v.doc.GSI
	info := GSIInfo{
		CPN: g.CPN, DFC: g.DFC, FrameRate: declaredFrameRate(g.DFC),
		DSC: displayCode(g.DSC), DisplayMode: displayModeName(g.DSC),
		CCT: g.CCT, LanguageCode: g.LanguageCode,
		TCS: displayCode(g.TCS), TCP: g.TCP, TCF: g.TCF,
		TND: g.TND, DSN: g.DSN, Programme: g.Programme, Episode: g.Episode,
	}
	if code, err := parseLanguageCode(g.LanguageCode); err == nil {
		info.Language = languageName(code)
	}
	return info
}

func (v *validator) collectRecords() {
	for i := range v.doc.Blocks {
		b := v.doc.Blocks[i]
		key := subtitleKey{SGN: b.SGN, SN: b.SN}
		rec := v.byKey[key]
		if rec == nil {
			rec = &subtitleRecord{Key: key, Header: -1}
			v.byKey[key] = rec
			v.records = append(v.records, rec)
		}
		rec.BlockIndex = append(rec.BlockIndex, i)
		if b.isTextBlock() {
			if rec.Header < 0 {
				rec.Header = i
			}
			rec.TextIndex = append(rec.TextIndex, i)
		}
	}
}

func (v *validator) fillActual() {
	v.report.Actual.TTIBlocks = len(v.doc.Blocks)
	v.report.Actual.Subtitles = len(v.records)
	groups := make(map[uint8]bool)
	for _, rec := range v.records {
		groups[rec.Key.SGN] = true
		if v.recordHasText(rec) {
			v.report.Actual.NonEmptySubtitles++
		} else {
			v.report.Actual.EmptySubtitles++
		}
	}
	v.report.Actual.SubtitleGroups = len(groups)

	seen := make(map[[TTIBlockSize]byte]int)
	for _, b := range v.doc.Blocks {
		if _, ok := seen[b.raw]; ok {
			v.report.Actual.DuplicateBlocks++
			continue
		}
		seen[b.raw] = b.Index
	}
}

func (v *validator) checkStructure() {
	switch {
	case v.doc.Size == 0:
		v.addGSI(SeverityError, CategoryStructure, "file",
			"the file is empty",
			"Provide a 1024-byte GSI block followed by complete 128-byte TTI blocks.")
	case !v.doc.GSIComplete:
		v.addGSI(SeverityError, CategoryStructure, "GSI",
			fmt.Sprintf("GSI block is truncated: got %d of %d bytes", v.doc.Size, GSIBlockSize),
			"Restore the complete 1024-byte GSI block. Do not pad it with invented field values.")
	}
	if v.doc.Trailing > 0 {
		index := len(v.doc.Blocks)
		ref := &BlockRef{
			Index: index, Number: index + 1,
			Offset: int64(GSIBlockSize + index*TTIBlockSize),
		}
		v.addBlock(ref, SeverityError, CategoryStructure, "TTI",
			fmt.Sprintf("file ends with an incomplete TTI block (%d of %d bytes)",
				v.doc.Trailing, TTIBlockSize),
			"Restore the missing bytes or remove the isolated fragment; keep all complete blocks intact.")
	}
}

func (v *validator) checkGSI() {
	g := v.doc.GSI
	if !v.doc.GSIComplete {
		return
	}
	if !validCPN(g.CPN) {
		v.addGSI(SeverityError, CategoryGSI, "CPN",
			fmt.Sprintf("code page %q is not one of 437, 850, 860, 863 or 865", g.CPN),
			"Use a code page defined by EBU Tech 3264 for GSI text, normally 850.")
	}
	if v.fps == 0 {
		v.addGSI(SeverityError, CategoryFormat, "DFC",
			fmt.Sprintf("disk format code %q is neither STL25.01 nor STL30.01", g.DFC),
			"Set DFC to STL25.01 for 25 fps or STL30.01 for 30/29.97 fps.")
	}
	if displayModeName(g.DSC) == "" {
		v.addGSI(SeverityError, CategoryDisplay, "DSC",
			fmt.Sprintf("display standard code 0x%02X is not 0x30, 0x31 or 0x32", g.DSC),
			"Use DSC '0' (open subtitling), '1' (Level-1 teletext) or '2' (Level-2 teletext).")
	}
	if !validCCT(g.CCT) {
		v.addGSI(SeverityError, CategoryLanguage, "CCT",
			fmt.Sprintf("character code table %q is not 00-04", g.CCT),
			"Use CCT 00 (Latin), 01 (Cyrillic), 02 (Arabic), 03 (Greek) or 04 (Hebrew).")
	}
	if code, err := parseLanguageCode(g.LanguageCode); err != nil {
		v.addGSI(SeverityError, CategoryLanguage, "LC", err.Error(),
			"Set LC to two hexadecimal digits in the range 00h-7Fh, e.g. 09h for English.")
	} else {
		v.language = &code
	}
	if g.TCS != '0' && g.TCS != '1' {
		v.addGSI(SeverityError, CategoryGSI, "TCS",
			fmt.Sprintf("time code status 0x%02X is not '0' or '1'", g.TCS),
			"Set TCS to '1' when GSI and TTI time codes are intended for use, otherwise '0'.")
	}
	if g.TCP != "" {
		v.checkGSITimecode("TCP", g.TCP)
	}
	if g.TCF != "" {
		v.checkGSITimecode("TCF", g.TCF)
	}

	v.report.Declared = Declared{
		TNB: parseCount(g.TNB, 99999),
		TNS: parseCount(g.TNS, 99999),
		TNG: parseCount(g.TNG, 999),
		MNC: parseCount(g.MNC, 99),
		MNR: parseCount(g.MNR, 99),
	}
	v.checkCountField("TNB", g.TNB, v.report.Declared.TNB, 99999)
	v.checkCountField("TNS", g.TNS, v.report.Declared.TNS, 99999)
	v.checkCountField("TNG", g.TNG, v.report.Declared.TNG, 999)
	v.checkCountField("MNC", g.MNC, v.report.Declared.MNC, 99)
	v.checkCountField("MNR", g.MNR, v.report.Declared.MNR, 99)

	tnd := parseCount(g.TND, 9)
	dsn := parseCount(g.DSN, 9)
	if tnd == nil || *tnd < 1 {
		v.addGSI(SeverityError, CategoryFormat, "TND",
			fmt.Sprintf("total number of disks %q is not 1-9", g.TND),
			"Set TND to the number of disks in the set; a single-file set uses 1.")
	}
	if dsn == nil || *dsn < 1 || (tnd != nil && *dsn > *tnd) {
		v.addGSI(SeverityError, CategoryFormat, "DSN",
			fmt.Sprintf("disk sequence number %q is not in 1..TND", g.DSN),
			"Set DSN to the one-based position of this disk within TND.")
	}
	if tnd != nil && *tnd > 1 {
		v.addGSI(SeverityWarning, CategoryCount, "TNB/TNS/TNG",
			fmt.Sprintf("TND=%d declares a multi-disk set; local TTI counts cannot prove the global totals", *tnd),
			"Verify global TNB, TNS and TNG across the complete disk set rather than this file alone.")
	}
}

func (v *validator) checkCountField(name, raw string, value *int, max int) {
	if value == nil {
		v.addGSI(SeverityError, CategoryCount, name,
			fmt.Sprintf("%s %q is not a decimal count in 0-%d", name, raw, max),
			fmt.Sprintf("Write %s as a zero-padded decimal number in 0-%d.", name, max))
		return
	}
	if (name == "MNC" || name == "MNR") && *value == 0 {
		v.addGSI(SeverityError, CategoryCount, name, name+" must be at least 1",
			"Set the display limit to a positive value consistent with the display standard.")
	}
}

func (v *validator) checkCounts() {
	if !v.doc.GSIComplete {
		return
	}
	tnd := parseCount(v.doc.GSI.TND, 9)
	if tnd != nil && *tnd > 1 {
		return
	}
	if d := v.report.Declared.TNB; d != nil && *d != v.report.Actual.TTIBlocks {
		v.addGSI(SeverityError, CategoryCount, "TNB",
			fmt.Sprintf("declares %d TTI blocks but the file contains %d", *d, v.report.Actual.TTIBlocks),
			fmt.Sprintf("Set TNB to %d without adding, deleting or truncating text.", v.report.Actual.TTIBlocks))
	}
	if d := v.report.Declared.TNS; d != nil && *d != v.report.Actual.Subtitles {
		v.addGSI(SeverityError, CategoryCount, "TNS",
			fmt.Sprintf("declares %d subtitles but %d subtitle records exist",
				*d, v.report.Actual.Subtitles),
			fmt.Sprintf("Set TNS to %d after resolving empty and duplicate subtitle records.", v.report.Actual.Subtitles))
	}
	if d := v.report.Declared.TNG; d != nil && *d != v.report.Actual.SubtitleGroups {
		v.addGSI(SeverityError, CategoryCount, "TNG",
			fmt.Sprintf("declares %d subtitle groups but blocks use %d",
				*d, v.report.Actual.SubtitleGroups),
			fmt.Sprintf("Set TNG to %d to match the SGN values actually used.", v.report.Actual.SubtitleGroups))
	}
}

func (v *validator) checkLanguage() {
	if v.language == nil || !validCCT(v.cct) {
		return
	}
	expected, ok := expectedCCT(*v.language)
	if ok && expected != v.cct {
		name := languageName(*v.language)
		v.addGSI(SeverityWarning, CategoryLanguage, "LC/CCT",
			fmt.Sprintf("LC %02Xh (%s) normally uses CCT %s, but CCT %s is declared",
				*v.language, name, expected, v.cct),
			fmt.Sprintf("Confirm that the TTI text encoding and language agree; use CCT %s for %s or correct LC.",
				expected, name))
	}
}

func (v *validator) checkGSITimecode(field, raw string) {
	tc, err := parseTimecodeString(raw)
	if err != nil {
		v.addGSI(SeverityError, CategoryTimecode, field, err.Error(),
			"Write the time code as eight ASCII digits HHMMSSFF.")
		return
	}
	if msg := timecodeRangeError(tc, v.fps, false); msg != "" {
		v.addGSI(SeverityError, CategoryTimecode, field, msg,
			"Use minutes and seconds 0-59 and a frame number below the DFC frame rate.")
	}
}

func (v *validator) addGSI(severity, category, field, message, suggestion string) {
	v.report.Issues = append(v.report.Issues, Issue{
		Category: category, Severity: severity, Field: field,
		Message: message, Suggestion: suggestion,
	})
}

func (v *validator) addBlock(ref *BlockRef, severity, category, field, message, suggestion string) {
	v.report.Issues = append(v.report.Issues, Issue{
		Block: ref, Category: category, Severity: severity, Field: field,
		Message: message, Suggestion: suggestion,
	})
}

func (v *validator) addTTI(t TTI, severity, category, field, message, suggestion string) {
	v.addBlock(blockRef(t), severity, category, field, message, suggestion)
}

func displayCode(b byte) string {
	if b >= 0x20 && b <= 0x7E {
		return string([]byte{b})
	}
	return fmt.Sprintf("0x%02X", b)
}

func declaredFrameRate(dfc string) int {
	switch dfc {
	case "STL25.01":
		return 25
	case "STL30.01":
		return 30
	default:
		return 0
	}
}

func validCPN(s string) bool {
	switch s {
	case "437", "850", "860", "863", "865":
		return true
	default:
		return false
	}
}

func validCCT(s string) bool {
	return s == "00" || s == "01" || s == "02" || s == "03" || s == "04"
}

func parseCount(raw string, max int) *int {
	s := strings.TrimSpace(raw)
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n > max {
		return nil
	}
	return &n
}

func parseTimecodeString(s string) (Timecode, error) {
	if len(s) != 8 {
		return Timecode{}, fmt.Errorf("time code %q is not eight digits", s)
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return Timecode{}, fmt.Errorf("time code %q is not eight digits", s)
		}
	}
	num := func(a, b int) uint8 { return (s[a]-'0')*10 + (s[b] - '0') }
	return Timecode{Hours: num(0, 1), Minutes: num(2, 3), Seconds: num(4, 5), Frames: num(6, 7)}, nil
}

func timecodeRangeError(t Timecode, fps int, checkHours bool) string {
	if checkHours && t.Hours > 23 {
		return fmt.Sprintf("%s has hours above 23", t)
	}
	if t.Minutes > 59 {
		return fmt.Sprintf("%s has minutes above 59", t)
	}
	if t.Seconds > 59 {
		return fmt.Sprintf("%s has seconds above 59", t)
	}
	if fps > 0 && int(t.Frames) >= fps {
		return fmt.Sprintf("%s has frame %d but DFC allows only 0-%d", t, t.Frames, fps-1)
	}
	if fps == 0 && t.Frames > 29 {
		return fmt.Sprintf("%s has frame %d but EBU STL supports at most 30 fps", t, t.Frames)
	}
	return ""
}

func (v *validator) recordHasText(rec *subtitleRecord) bool {
	for _, idx := range rec.TextIndex {
		content := visibleContent(v.doc.Blocks[idx].Text[:], v.doc.Blocks[idx].isLastBlock())
		if hasVisible(content, v.cct) {
			return true
		}
	}
	return false
}

func hasVisible(content []byte, cct string) bool {
	for i := 0; i < len(content); {
		b := content[i]
		switch {
		case b == lineBreak || b == unusedSpace:
			i++
		case b <= 0x1F:
			i++
		case b >= 0x80 && b <= 0x9F:
			i++
		case cct == "00" && b >= 0xC1 && b <= 0xCF:
			i++
		case b == ' ':
			i++
		default:
			return true
		}
	}
	return false
}

func visibleContent(field []byte, last bool) []byte {
	end := len(field)
	if i := indexByte(field, unusedSpace); i >= 0 {
		end = i
	} else if last {
		end = len(field)
	}
	return field[:end]
}

func indexByte(b []byte, want byte) int {
	for i, got := range b {
		if got == want {
			return i
		}
	}
	return -1
}

func validUTF8Text(b []byte) (bool, int) {
	if len(b) == 0 || !utf8.Valid(b) {
		return false, 0
	}
	n := 0
	for _, r := range string(b) {
		if r > 0xFF {
			n++
		}
	}
	return n > 0, n
}
