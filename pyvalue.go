// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// Extra arguments are dynamically typed, as in the Python API: nil, bool, numbers (int or
// float64, as encoding/json decodes them), string, []any or map[string]any. The helpers below give
// them Python's semantics (truthiness, str(), attribute errors on the wrong type).

// MarkdownError is the library's own error: an invalid safe mode, or the "link-patterns" extra
// without link patterns. Its message is the one the Python library words.
type MarkdownError struct {
	Message string
}

func (e *MarkdownError) Error() string { return e.Message }

// Error is any other failure of a conversion, for inputs or extra arguments the library cannot
// handle. Type names the failure as Python names it ("TypeError", "AttributeError", ...).
type Error struct {
	Type    string
	Message string
}

func (e *Error) Error() string { return e.Type + ": " + e.Message }

// raise aborts the conversion; New and Converter.Convert return the failure as an error.
func raise(typ, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if typ == "MarkdownError" {
		panic(&MarkdownError{Message: msg})
	}
	panic(&Error{Type: typ, Message: msg})
}

// typeName is Python's type(v).__name__ for an extra argument.
func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case int, int64:
		return "int"
	case float64:
		if isIntegral(v) {
			return "int"
		}
		return "float"
	case string:
		return "str"
	case []any:
		return "list"
	case map[string]any:
		return "dict"
	}
	return fmt.Sprintf("%T", v)
}

func isIntegral(v any) bool {
	f, ok := v.(float64)
	return ok && f == math.Trunc(f) && math.Abs(f) < 1<<53
}

// asInt returns v as an integer when Python would treat it as an int (bools included).
func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		if isIntegral(x) {
			return int(x), true
		}
	}
	return 0, false
}

// truthy is Python's bool(v).
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

// optGet is options.get(key, def) on an extra's options; options that are not a dict raise
// AttributeError as in Python.
func optGet(opts any, key string, def any) any {
	m, ok := opts.(map[string]any)
	if !ok {
		raise("AttributeError", "'%s' object has no attribute 'get'", typeName(opts))
	}
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

// pyStr is Python's str(v).
func pyStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	}
	return pyRepr(v)
}

// pyRepr is Python's repr(v).
func pyRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if isIntegral(x) {
			return strconv.FormatInt(int64(x), 10)
		}
		return pyFloatRepr(x)
	case string:
		return pyunicode.Repr(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = pyRepr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = pyunicode.Repr(k) + ": " + pyRepr(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(v)
}

func pyFloatRepr(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	s := strconv.FormatFloat(f, 'r', -1, 64)
	abs := math.Abs(f)
	if abs >= 1e16 || abs != 0 && abs < 1e-4 {
		s = strconv.FormatFloat(f, 'e', -1, 64)
		mant, exp, _ := strings.Cut(s, "e")
		sign := exp[0]
		exp = strings.TrimLeft(exp[1:], "0")
		if len(exp) < 2 {
			exp = strings.Repeat("0", 2-len(exp)) + exp
		}
		return mant + "e" + string(sign) + exp
	}
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}

// pyFormat is Python's printf-style "format % args" for the conversions markdown2's templates
// can meet. It raises TypeError and ValueError as Python does.
func pyFormat(format string, args ...any) string {
	var b strings.Builder
	argi := 0
	next := func() any {
		if argi >= len(args) {
			raise("TypeError", "not enough arguments for format string")
		}
		argi++
		return args[argi-1]
	}
	rs := []rune(format)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '%' {
			b.WriteRune(rs[i])
			continue
		}
		i++
		if i >= len(rs) {
			raise("ValueError", "incomplete format")
		}
		if rs[i] == '(' {
			raise("TypeError", "format requires a mapping")
		}
		var flags string
		for i < len(rs) && strings.ContainsRune("#0- +", rs[i]) {
			flags += string(rs[i])
			i++
		}
		width := -1
		if i < len(rs) && rs[i] == '*' {
			w, ok := asInt(next())
			if !ok {
				raise("TypeError", "* wants int")
			}
			width = w
			if width < 0 {
				flags += "-"
				width = -width
			}
			i++
		} else {
			for i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
				if width < 0 {
					width = 0
				}
				width = width*10 + int(rs[i]-'0')
				i++
			}
		}
		prec := -1
		if i < len(rs) && rs[i] == '.' {
			i++
			prec = 0
			if i < len(rs) && rs[i] == '*' {
				p, ok := asInt(next())
				if !ok {
					raise("TypeError", "* wants int")
				}
				prec = p
				i++
			} else {
				for i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
					prec = prec*10 + int(rs[i]-'0')
					i++
				}
			}
		}
		for i < len(rs) && (rs[i] == 'h' || rs[i] == 'l' || rs[i] == 'L') {
			i++
		}
		if i >= len(rs) {
			raise("ValueError", "incomplete format")
		}
		conv := rs[i]
		if conv == '%' {
			b.WriteByte('%')
			continue
		}
		var body string
		numeric := false
		switch conv {
		case 's':
			body = pyStr(next())
			if prec >= 0 {
				body = truncRunes(body, prec)
			}
		case 'r', 'a':
			body = pyRepr(next())
			if conv == 'a' {
				body = asciiRepr(body)
			}
			if prec >= 0 {
				body = truncRunes(body, prec)
			}
		case 'c':
			v := next()
			switch x := v.(type) {
			case string:
				if len([]rune(x)) != 1 {
					raise("TypeError", "%%c requires int or char")
				}
				body = x
			default:
				n, ok := asInt(v)
				if !ok {
					raise("TypeError", "%%c requires int or char")
				}
				if n < 0 || n > 0x10ffff {
					raise("OverflowError", "%%c arg not in range(0x110000)")
				}
				body = string(rune(n))
			}
		case 'd', 'i', 'u', 'x', 'X', 'o':
			v := next()
			n, ok := asInt(v)
			if !ok {
				if f, isF := v.(float64); isF && (conv == 'd' || conv == 'i' || conv == 'u') {
					n, ok = int(f), true
				}
			}
			if !ok {
				if conv == 'd' || conv == 'i' || conv == 'u' {
					raise("TypeError", "%%%c format: a real number is required, not %s", conv, typeName(v))
				}
				raise("TypeError", "%%%c format: an integer is required, not %s", conv, typeName(v))
			}
			body, numeric = formatInt(n, conv, flags, prec), true
		case 'e', 'E', 'f', 'F', 'g', 'G':
			v := next()
			var f float64
			if n, ok := asInt(v); ok {
				f = float64(n)
			} else if x, ok := v.(float64); ok {
				f = x
			} else {
				raise("TypeError", "must be real number, not %s", typeName(v))
			}
			body, numeric = formatFloat(f, conv, flags, prec), true
		default:
			raise("ValueError", "unsupported format character '%c' (0x%x) at index %d", conv, conv, i)
		}
		body = pad(body, width, flags, numeric)
		b.WriteString(body)
	}
	if argi < len(args) {
		raise("TypeError", "not all arguments converted during string formatting")
	}
	return b.String()
}

func truncRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}

func asciiRepr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case r <= 0xff:
			fmt.Fprintf(&b, "\\x%02x", r)
		case r <= 0xffff:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			fmt.Fprintf(&b, "\\U%08x", r)
		}
	}
	return b.String()
}

func signPrefix(neg bool, flags string) string {
	switch {
	case neg:
		return "-"
	case strings.Contains(flags, "+"):
		return "+"
	case strings.Contains(flags, " "):
		return " "
	}
	return ""
}

func formatInt(n int, conv rune, flags string, prec int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var digits, prefix string
	switch conv {
	case 'x':
		digits = strconv.FormatInt(int64(n), 16)
		if strings.Contains(flags, "#") {
			prefix = "0x"
		}
	case 'X':
		digits = strings.ToUpper(strconv.FormatInt(int64(n), 16))
		if strings.Contains(flags, "#") {
			prefix = "0X"
		}
	case 'o':
		digits = strconv.FormatInt(int64(n), 8)
		if strings.Contains(flags, "#") {
			prefix = "0o"
		}
	default:
		digits = strconv.Itoa(n)
	}
	if prec > len(digits) {
		digits = strings.Repeat("0", prec-len(digits)) + digits
	}
	return signPrefix(neg, flags) + prefix + digits
}

func formatFloat(f float64, conv rune, flags string, prec int) string {
	if prec < 0 {
		prec = 6
	}
	neg := math.Signbit(f) && !math.IsNaN(f)
	f = math.Abs(f)
	var s string
	switch {
	case math.IsInf(f, 0):
		s = "inf"
	case math.IsNaN(f):
		s = "nan"
	default:
		switch conv {
		case 'f', 'F':
			s = strconv.FormatFloat(f, 'f', prec, 64)
		case 'e', 'E':
			s = pyExp(strconv.FormatFloat(f, 'e', prec, 64))
		case 'g', 'G':
			if prec == 0 {
				prec = 1
			}
			exp := 0
			if f != 0 {
				exp = int(math.Floor(math.Log10(f)))
				// correct for rounding at the precision
				r, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'e', prec-1, 64), 64)
				exp = int(math.Floor(math.Log10(r)))
			}
			if exp < -4 || exp >= prec {
				s = pyExp(strconv.FormatFloat(f, 'e', prec-1, 64))
				if !strings.Contains(flags, "#") {
					mant, e, _ := strings.Cut(s, "e")
					if strings.Contains(mant, ".") {
						mant = strings.TrimRight(strings.TrimRight(mant, "0"), ".")
					}
					s = mant + "e" + e
				}
			} else {
				s = strconv.FormatFloat(f, 'f', prec-1-exp, 64)
				if !strings.Contains(flags, "#") && strings.Contains(s, ".") {
					s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
				}
			}
		}
	}
	if conv == 'E' || conv == 'G' || conv == 'F' {
		s = strings.ToUpper(s)
	}
	return signPrefix(neg, flags) + s
}

// pyExp rewrites Go's exponent (e+5) in Python's two-digit minimum form (e+05).
func pyExp(s string) string {
	mant, exp, ok := strings.Cut(s, "e")
	if !ok {
		return s
	}
	sign := exp[:1]
	digits := strings.TrimLeft(exp[1:], "0")
	for len(digits) < 2 {
		digits = "0" + digits
	}
	return mant + "e" + sign + digits
}

func pad(body string, width int, flags string, numeric bool) string {
	n := len([]rune(body))
	if width <= n {
		return body
	}
	fill := width - n
	switch {
	case strings.Contains(flags, "-"):
		return body + strings.Repeat(" ", fill)
	case numeric && strings.Contains(flags, "0"):
		sign := ""
		if body != "" && strings.ContainsRune("+- ", rune(body[0])) {
			sign, body = body[:1], body[1:]
		}
		if strings.HasPrefix(body, "0x") || strings.HasPrefix(body, "0X") || strings.HasPrefix(body, "0o") {
			sign, body = sign+body[:2], body[2:]
		}
		return sign + strings.Repeat("0", fill) + body
	}
	return strings.Repeat(" ", fill) + body
}
