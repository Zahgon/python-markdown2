// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

// Package pyhtml implements html.unescape as Python's standard library defines it: HTML5 named
// references (with the longest-prefix rule for names written without a semicolon) and numeric
// references with the HTML5 replacements for invalid code points.
package pyhtml

import (
	"strings"
	"unicode/utf8"
)

var invalidCharrefs = map[int]string{
	0x00: "�", 0x0d: "\r", 0x80: "€", 0x81: "\x81", 0x82: "‚", 0x83: "ƒ", 0x84: "„",
	0x85: "…", 0x86: "†", 0x87: "‡", 0x88: "ˆ", 0x89: "‰", 0x8a: "Š", 0x8b: "‹",
	0x8c: "Œ", 0x8d: "\x8d", 0x8e: "Ž", 0x8f: "\x8f", 0x90: "\x90", 0x91: "‘", 0x92: "’",
	0x93: "“", 0x94: "”", 0x95: "•", 0x96: "–", 0x97: "—", 0x98: "˜", 0x99: "™",
	0x9a: "š", 0x9b: "›", 0x9c: "œ", 0x9d: "\x9d", 0x9e: "ž", 0x9f: "Ÿ",
}

func invalidCodepoint(n int) bool {
	switch {
	case 0x1 <= n && n <= 0x8, 0xe <= n && n <= 0x1f, 0x7f <= n && n <= 0x9f, 0xfdd0 <= n && n <= 0xfdef, n == 0xb:
		return true
	}
	return n&0xfffe == 0xfffe && n <= 0x10ffff
}

func isHex(c byte) bool { return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' }

func hexVal(c byte) int {
	switch {
	case c <= '9':
		return int(c - '0')
	case c >= 'a':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

// numeric decodes the digits of a numeric reference; values past the Unicode range saturate.
func numeric(digits string, base int) int {
	n := 0
	for i := 0; i < len(digits); i++ {
		n = n*base + hexVal(digits[i])
		if n > 0x10ffff {
			n = 0x110000
		}
	}
	return n
}

// Unescape replaces every character reference in s.
func Unescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	i := 0
	for {
		j := strings.IndexByte(s[i:], '&')
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		b.WriteString(s[i : i+j])
		i += j
		repl, n := reference(s[i+1:])
		if n == 0 {
			b.WriteByte('&')
			i++
			continue
		}
		b.WriteString(repl)
		i += 1 + n
	}
}

// reference decodes the reference that follows an ampersand; n is the number of bytes consumed
// (0 when nothing after the ampersand forms a reference).
func reference(s string) (string, int) {
	if strings.HasPrefix(s, "#") {
		base, start := 10, 1
		if len(s) > 1 && (s[1] == 'x' || s[1] == 'X') {
			base, start = 16, 2
		}
		end := start
		for end < len(s) && (base == 16 && isHex(s[end]) || base == 10 && '0' <= s[end] && s[end] <= '9') {
			end++
		}
		if end == start {
			return "", 0
		}
		num := numeric(s[start:end], base)
		if end < len(s) && s[end] == ';' {
			end++
		}
		if r, ok := invalidCharrefs[num]; ok {
			return r, end
		}
		if 0xd800 <= num && num <= 0xdfff || num > 0x10ffff {
			return "�", end
		}
		if invalidCodepoint(num) {
			return "", end
		}
		return string(rune(num)), end
	}
	// a name: up to 32 characters other than tab, newline, form feed, space, <, &, # and ;
	end, count := 0, 0
	for end < len(s) && count < 32 {
		r, size := utf8.DecodeRuneInString(s[end:])
		if r == '\t' || r == '\n' || r == '\f' || r == ' ' || r == '<' || r == '&' || r == '#' || r == ';' {
			break
		}
		end += size
		count++
	}
	if end == 0 {
		return "", 0
	}
	if end < len(s) && s[end] == ';' {
		end++
	}
	name := s[:end]
	if v, ok := entities[name]; ok {
		return v, end
	}
	runes := []rune(name)
	for x := len(runes) - 1; x > 1; x-- {
		if v, ok := entities[string(runes[:x])]; ok {
			return v + string(runes[x:]), end
		}
	}
	return "&" + name, end
}
