package stl

import (
	"fmt"
	"strconv"
	"strings"
)

// languageNames follows EBU Tech 3264 Appendix 3. Codes 2Ch-44h are
// reserved for national assignment and therefore have no fixed name.
var languageNames = map[uint8]string{
	0x00: "Unknown/not applicable",
	0x01: "Albanian",
	0x02: "Breton",
	0x03: "Catalan",
	0x04: "Croatian",
	0x05: "Welsh",
	0x06: "Czech",
	0x07: "Danish",
	0x08: "German",
	0x09: "English",
	0x0A: "Spanish",
	0x0B: "Esperanto",
	0x0C: "Estonian",
	0x0D: "Basque",
	0x0E: "Faroese",
	0x0F: "French",
	0x10: "Frisian",
	0x11: "Irish",
	0x12: "Gaelic",
	0x13: "Galician",
	0x14: "Icelandic",
	0x15: "Italian",
	0x16: "Lappish",
	0x17: "Latin",
	0x18: "Latvian",
	0x19: "Luxembourgian",
	0x1A: "Lithuanian",
	0x1B: "Hungarian",
	0x1C: "Maltese",
	0x1D: "Dutch",
	0x1E: "Norwegian",
	0x1F: "Occitan",
	0x20: "Polish",
	0x21: "Portuguese",
	0x22: "Romanian",
	0x23: "Romansh",
	0x24: "Serbian",
	0x25: "Slovak",
	0x26: "Slovenian",
	0x27: "Finnish",
	0x28: "Swedish",
	0x29: "Turkish",
	0x2A: "Flemish",
	0x2B: "Walloon",
	0x45: "Zulu",
	0x46: "Vietnamese",
	0x47: "Uzbek",
	0x48: "Urdu",
	0x49: "Ukrainian",
	0x4A: "Thai",
	0x4B: "Telugu",
	0x4C: "Tatar",
	0x4D: "Tamil",
	0x4E: "Tadzhik",
	0x4F: "Swahili",
	0x50: "Sranan Tongo",
	0x51: "Somali",
	0x52: "Sinhalese",
	0x53: "Shona",
	0x54: "Serbo-Croat",
	0x55: "Ruthenian",
	0x56: "Russian",
	0x57: "Quechua",
	0x58: "Pushtu",
	0x59: "Punjabi",
	0x5A: "Persian",
	0x5B: "Papiamento",
	0x5C: "Oriya",
	0x5D: "Nepali",
	0x5E: "Ndebele",
	0x5F: "Marathi",
	0x60: "Moldavian",
	0x61: "Malaysian",
	0x62: "Malagasy",
	0x63: "Macedonian",
	0x64: "Laotian",
	0x65: "Korean",
	0x66: "Khmer",
	0x67: "Kazakh",
	0x68: "Kannada",
	0x69: "Japanese",
	0x6A: "Indonesian",
	0x6B: "Hindi",
	0x6C: "Hebrew",
	0x6D: "Hausa",
	0x6E: "Gurani",
	0x6F: "Gujarati",
	0x70: "Greek",
	0x71: "Georgian",
	0x72: "Fulani",
	0x73: "Dari",
	0x74: "Churash",
	0x75: "Chinese",
	0x76: "Burmese",
	0x77: "Bulgarian",
	0x78: "Bengali",
	0x79: "Bielorussian",
	0x7A: "Bambora",
	0x7B: "Azerbaijani",
	0x7C: "Assamese",
	0x7D: "Armenian",
	0x7E: "Arabic",
	0x7F: "Amharic",
}

var cyrillicLanguages = map[uint8]bool{
	0x49: true, // Ukrainian
	0x56: true, // Russian
	0x63: true, // Macedonian
	0x67: true, // Kazakh
	0x77: true, // Bulgarian
	0x79: true, // Bielorussian
}

func parseLanguageCode(s string) (uint8, error) {
	if len(s) != 2 {
		return 0, fmt.Errorf("%q is not two hexadecimal digits", s)
	}
	code, err := strconv.ParseUint(s, 16, 8)
	if err != nil {
		return 0, fmt.Errorf("%q is not two hexadecimal digits", s)
	}
	if code > 0x7F {
		return 0, fmt.Errorf("%s is outside 00h-7Fh", strings.ToUpper(s))
	}
	return uint8(code), nil
}

func languageName(code uint8) string {
	if name, ok := languageNames[code]; ok {
		return name
	}
	return "reserved for national assignment"
}

// expectedCCT returns the character code table normally used by a language.
// Languages that may legitimately use more than one script return ok=false.
func expectedCCT(code uint8) (string, bool) {
	switch {
	case code == 0x24: // Serbian is also written in Cyrillic.
		return "", false
	case code >= 0x01 && code <= 0x2B:
		return "00", true
	case cyrillicLanguages[code]:
		return "01", true
	case code == 0x7E:
		return "02", true
	case code == 0x70:
		return "03", true
	case code == 0x6C:
		return "04", true
	default:
		return "", false
	}
}

func displayModeName(code byte) string {
	switch code {
	case '0':
		return "Open subtitling"
	case '1':
		return "Level-1 teletext"
	case '2':
		return "Level-2 teletext"
	default:
		return ""
	}
}
