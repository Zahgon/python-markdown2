// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// cp1252High maps the bytes 0x80-0x9F of Windows-1252 (0 marks an undefined byte).
var cp1252High = [32]rune{
	0x20AC, 0, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, 0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0, 0x017D, 0,
	0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0, 0x017E, 0x0178,
}

var encodingAliases = map[string]string{
	"utf_8": "utf_8", "utf8": "utf_8", "u8": "utf_8", "utf": "utf_8", "utf8_ucs2": "utf_8", "utf8_ucs4": "utf_8", "cp65001": "utf_8",
	"utf_8_sig": "utf_8_sig",
	"latin_1":   "latin_1", "latin1": "latin_1", "latin": "latin_1", "l1": "latin_1", "iso_8859_1": "latin_1",
	"iso8859_1": "latin_1", "8859": "latin_1", "cp819": "latin_1", "iso_ir_100": "latin_1", "csisolatin1": "latin_1",
	"iso_8859_1_1987": "latin_1", "ibm819": "latin_1",
	"ascii": "ascii", "us_ascii": "ascii", "646": "ascii", "us": "ascii", "ansi_x3.4_1968": "ascii", "ansi_x3_4_1968": "ascii",
	"ansi_x3.4_1986": "ascii", "cp367": "ascii", "csascii": "ascii", "ibm367": "ascii", "iso646_us": "ascii",
	"iso_646.irv_1991": "ascii", "iso_ir_6": "ascii",
	"cp1252": "cp1252", "windows_1252": "cp1252",
	"utf_16": "utf_16", "utf16": "utf_16", "u16": "utf_16",
	"utf_16_le": "utf_16_le", "utf_16le": "utf_16_le", "unicodelittleunmarked": "utf_16_le",
	"utf_16_be": "utf_16_be", "utf_16be": "utf_16_be", "unicodebigunmarked": "utf_16_be",
}

// normalizeEncoding is Python's encodings.normalize_encoding followed by lowercasing.
func normalizeEncoding(name string) string {
	var b strings.Builder
	punct := false
	for _, r := range name {
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.') {
			if punct && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			punct = false
		} else {
			punct = true
		}
	}
	return strings.ToLower(b.String())
}

// Decode decodes data with a Python codec name ("" means UTF-8) and translates line endings
// as Python's text mode does.
func Decode(data []byte, encoding string) (string, error) {
	if encoding == "" {
		encoding = "utf-8"
	}
	codec, ok := encodingAliases[normalizeEncoding(encoding)]
	if !ok {
		return "", &Error{Type: "LookupError", Message: "unknown encoding: " + encoding}
	}
	var text string
	decodeErr := func(pos int, reason string) error {
		return &Error{Type: "UnicodeDecodeError", Message: fmt.Sprintf("'%s' codec can't decode byte 0x%02x in position %d: %s",
			strings.ReplaceAll(codec, "_", "-"), data[pos], pos, reason)}
	}
	switch codec {
	case "utf_8", "utf_8_sig":
		if !utf8.Valid(data) {
			for i := 0; i < len(data); {
				r, size := utf8.DecodeRune(data[i:])
				if r == utf8.RuneError && size <= 1 {
					return "", decodeErr(i, "invalid start byte")
				}
				i += size
			}
		}
		text = string(data)
		if codec == "utf_8_sig" {
			text = strings.TrimPrefix(text, "\ufeff")
		}
	case "latin_1":
		rs := make([]rune, len(data))
		for i, b := range data {
			rs[i] = rune(b)
		}
		text = string(rs)
	case "ascii":
		for i, b := range data {
			if b >= 0x80 {
				return "", decodeErr(i, "ordinal not in range(128)")
			}
		}
		text = string(data)
	case "cp1252":
		rs := make([]rune, len(data))
		for i, b := range data {
			switch {
			case b >= 0x80 && b < 0xa0:
				if cp1252High[b-0x80] == 0 {
					return "", decodeErr(i, "character maps to <undefined>")
				}
				rs[i] = cp1252High[b-0x80]
			default:
				rs[i] = rune(b)
			}
		}
		text = string(rs)
	default:
		bigEndian := codec == "utf_16_be"
		body := data
		if codec == "utf_16" && len(data) >= 2 {
			switch {
			case data[0] == 0xff && data[1] == 0xfe:
				body = data[2:]
			case data[0] == 0xfe && data[1] == 0xff:
				body, bigEndian = data[2:], true
			}
		}
		if len(body)%2 != 0 {
			return "", &Error{Type: "UnicodeDecodeError", Message: "truncated data"}
		}
		units := make([]uint16, len(body)/2)
		for i := range units {
			if bigEndian {
				units[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			} else {
				units[i] = uint16(body[2*i+1])<<8 | uint16(body[2*i])
			}
		}
		for i := 0; i < len(units); i++ {
			if u := units[i]; u >= 0xd800 && u < 0xe000 {
				if u >= 0xdc00 || i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] >= 0xe000 {
					return "", &Error{Type: "UnicodeDecodeError", Message: "illegal UTF-16 surrogate"}
				}
				i++
			}
		}
		text = string(utf16.Decode(units))
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n"), nil
}
