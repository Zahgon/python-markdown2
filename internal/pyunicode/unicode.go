// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

// Package pyunicode implements the Unicode character predicates and string transformations of
// Python's str type (isalnum, isspace, lower, splitlines, strip, NFKD normalisation, ...),
// which markdown2 relies on and which differ in places from Go's unicode package.
package pyunicode

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type rng struct{ lo, hi rune }

func inRanges(r rune, table []rng) bool {
	i := sort.Search(len(table), func(i int) bool { return table[i].hi >= r })
	return i < len(table) && table[i].lo <= r
}

// IsAlnum reports whether r is alphanumeric as Python's str.isalnum defines it.
func IsAlnum(r rune) bool {
	if r < 0x80 {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9'
	}
	return inRanges(r, alnumRanges)
}

// IsWord reports whether r matches Python's unicode \w.
func IsWord(r rune) bool { return r == '_' || IsAlnum(r) }

// IsDecimal reports whether r is a decimal digit (str.isdecimal, regex \d).
func IsDecimal(r rune) bool {
	if r < 0x80 {
		return '0' <= r && r <= '9'
	}
	return inRanges(r, decimalRanges)
}

// IsSpace reports whether r is white space as Python's str.isspace defines it.
func IsSpace(r rune) bool {
	if r < 0x80 {
		return r == ' ' || '\t' <= r && r <= '\r' || 0x1c <= r && r <= 0x1f
	}
	return inRanges(r, spaceRanges)
}

// IsPrintable reports whether r is printable as Python's str.isprintable defines it.
func IsPrintable(r rune) bool { return inRanges(r, printableRanges) }

// DecimalValue returns the value of the decimal digit r.
func DecimalValue(r rune) int {
	if '0' <= r && r <= '9' {
		return int(r - '0')
	}
	return int(decimalValue[r])
}

// SimpleLower is the one-character lowercase mapping used by the regex engine.
func SimpleLower(r rune) rune {
	if l, ok := simpleLower[r]; ok {
		return l
	}
	return r
}

// SimpleUpper is the one-character uppercase mapping used by the regex engine.
func SimpleUpper(r rune) rune { return unicode.ToUpper(r) }

// IgnoreCaseFixes lists the other lowercase characters that are case-insensitively equal to the
// lowercase character r.
func IgnoreCaseFixes(r rune) []rune { return ignoreCaseFixes[r] }

// IsCased reports whether a case-insensitive comparison can match r with another character.
func IsCased(r rune) bool {
	return SimpleLower(r) != r || SimpleUpper(r) != r || unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
}

func isCasedLetter(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
}

func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0xA8, 0xAD, 0xAF, 0xB4, 0xB7, 0xB8, 0x2018, 0x2019, 0x2024, 0x2027, 0xFE13,
		0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// finalSigma reports whether the capital sigma at rs[i] ends a word (Unicode Final_Sigma context).
func finalSigma(rs []rune, i int) bool {
	j := i - 1
	for j >= 0 && isCaseIgnorable(rs[j]) {
		j--
	}
	if j < 0 || !isCasedLetter(rs[j]) {
		return false
	}
	j = i + 1
	for j < len(rs) && isCaseIgnorable(rs[j]) {
		j++
	}
	return j == len(rs) || !isCasedLetter(rs[j])
}

// Lower is Python's str.lower: full case mappings and the final sigma rule.
func Lower(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s)
	}
	// Bytes that are not UTF-8 (undecodable input kept as is) pass through unchanged.
	var rs []rune
	var raw []int // byte value for an undecodable byte, -1 otherwise
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			rs, raw = append(rs, r), append(raw, int(s[i]))
		} else {
			rs, raw = append(rs, r), append(raw, -1)
		}
		i += size
	}
	var b strings.Builder
	for i, r := range rs {
		switch {
		case raw[i] >= 0:
			b.WriteByte(byte(raw[i]))
		case r == 0x3A3:
			if finalSigma(rs, i) {
				b.WriteRune(0x3C2)
			} else {
				b.WriteRune(0x3C3)
			}
		default:
			if l, ok := lowerFull[r]; ok {
				b.WriteString(l)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// TrimSpace is Python's str.strip() without arguments.
func TrimSpace(s string) string { return strings.TrimFunc(s, IsSpace) }

// TrimLeftSpace is Python's str.lstrip() without arguments.
func TrimLeftSpace(s string) string { return strings.TrimLeftFunc(s, IsSpace) }

// TrimRightSpace is Python's str.rstrip() without arguments.
func TrimRightSpace(s string) string { return strings.TrimRightFunc(s, IsSpace) }

// Fields is Python's str.split() without arguments.
func Fields(s string) []string { return strings.FieldsFunc(s, IsSpace) }

// isLineBreak reports whether r ends a line for str.splitlines.
func isLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// SplitLines is Python's str.splitlines(keepends).
func SplitLines(s string, keepends bool) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !isLineBreak(r) {
			i += size
			continue
		}
		end := i + size
		if r == '\r' && end < len(s) && s[end] == '\n' {
			end++
		}
		if keepends {
			out = append(out, s[start:end])
		} else {
			out = append(out, s[start:i])
		}
		start, i = end, end
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// NFKD returns the compatibility decomposition of s with its combining marks canonically ordered.
func NFKD(s string) string {
	var rs []rune
	for _, r := range s {
		switch {
		case 0xAC00 <= r && r <= 0xD7A3:
			idx := r - 0xAC00
			rs = append(rs, 0x1100+idx/588, 0x1161+(idx%588)/28)
			if t := idx % 28; t != 0 {
				rs = append(rs, 0x11A7+t)
			}
		default:
			if d, ok := nfkd[r]; ok {
				rs = append(rs, []rune(d)...)
			} else {
				rs = append(rs, r)
			}
		}
	}
	for i := 0; i < len(rs); {
		if combiningClass[rs[i]] == 0 {
			i++
			continue
		}
		j := i
		for j < len(rs) && combiningClass[rs[j]] != 0 {
			j++
		}
		run := rs[i:j]
		sort.SliceStable(run, func(a, b int) bool { return combiningClass[run[a]] < combiningClass[run[b]] })
		i = j
	}
	return string(rs)
}

// ParseInt is Python's int() of a str: surrounding white space, a sign, single underscores
// between digits and any Unicode decimal digits are accepted.
func ParseInt(s string) (int, bool) {
	t := TrimSpace(s)
	neg := false
	if t != "" && (t[0] == '+' || t[0] == '-') {
		neg = t[0] == '-'
		t = t[1:]
	}
	n, digits, lastUnderscore := 0, 0, true
	for _, r := range t {
		switch {
		case r == '_' && !lastUnderscore:
			lastUnderscore = true
		case IsDecimal(r):
			if n < 1<<53 {
				n = n*10 + DecimalValue(r)
			}
			digits++
			lastUnderscore = false
		default:
			return 0, false
		}
	}
	if digits == 0 || lastUnderscore {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

// Repr returns Python's repr() of a str.
func Repr(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			b.WriteString(hexEscape(`\x`, r, 2))
		case r < 0x7f:
			b.WriteRune(r)
		case IsPrintable(r):
			b.WriteRune(r)
		case r <= 0xff:
			b.WriteString(hexEscape(`\x`, r, 2))
		case r <= 0xffff:
			b.WriteString(hexEscape(`\u`, r, 4))
		default:
			b.WriteString(hexEscape(`\U`, r, 8))
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// BytesRepr returns Python's repr() of a bytes object.
func BytesRepr(data []byte) string {
	quote := byte('\'')
	if strings.IndexByte(string(data), '\'') >= 0 && strings.IndexByte(string(data), '"') < 0 {
		quote = '"'
	}
	var b strings.Builder
	b.WriteString("b")
	b.WriteByte(quote)
	for _, c := range data {
		switch {
		case c == quote || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20 || c >= 0x7f:
			b.WriteString(hexEscape(`\x`, rune(c), 2))
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func hexEscape(prefix string, r rune, width int) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = digits[r&0xf]
		r >>= 4
	}
	return prefix + string(buf)
}
