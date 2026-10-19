// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package pyre

import (
	"fmt"
	"strconv"
	"strings"
)

// template is a parsed replacement template: literal pieces interleaved with group references.
type template struct {
	parts  []string
	groups []int // groups[i] >= 0 means parts[i] is replaced by that group
}

func (t *template) expand(m *Match) string {
	var b strings.Builder
	for i, part := range t.parts {
		if t.groups[i] >= 0 {
			b.WriteString(m.Group(t.groups[i]))
		} else {
			b.WriteString(part)
		}
	}
	return b.String()
}

func isOct(r rune) bool { return '0' <= r && r <= '7' }

func isDigit(r rune) bool { return '0' <= r && r <= '9' }

// parseTemplate follows Python's re._parser.parse_template.
func (re *Regexp) parseTemplate(src string) (*template, error) {
	t := &template{}
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			t.parts = append(t.parts, lit.String())
			t.groups = append(t.groups, -1)
			lit.Reset()
		}
	}
	addGroup := func(g int) {
		flush()
		t.parts = append(t.parts, "")
		t.groups = append(t.groups, g)
	}
	rs := []rune(src)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if c != '\\' {
			lit.WriteRune(c)
			continue
		}
		if i+1 >= len(rs) {
			return nil, &Error{Msg: "bad escape (end of pattern)", Pattern: src, Pos: i}
		}
		start := i
		i++
		c = rs[i]
		switch {
		case c == 'g':
			if i+1 >= len(rs) || rs[i+1] != '<' {
				return nil, &Error{Msg: "missing <", Pattern: src, Pos: i + 1}
			}
			j := i + 2
			for j < len(rs) && rs[j] != '>' {
				j++
			}
			if j >= len(rs) {
				return nil, &Error{Msg: "missing >, unterminated name", Pattern: src, Pos: i + 2}
			}
			name := string(rs[i+2 : j])
			if name == "" {
				return nil, &Error{Msg: "missing group name", Pattern: src, Pos: i + 2}
			}
			var g int
			if allDigits(name) {
				n, err := strconv.Atoi(name)
				if err != nil || n > re.ngroups {
					return nil, &Error{Msg: fmt.Sprintf("invalid group reference %s", name), Pattern: src, Pos: i + 2}
				}
				g = n
			} else {
				if !isIdentifier(name) {
					return nil, &Error{Msg: fmt.Sprintf("bad character in group name %s", strconv.Quote(name)), Pattern: src, Pos: i + 2}
				}
				n, ok := re.names[name]
				if !ok {
					return nil, &Error{Msg: fmt.Sprintf("unknown group name '%s'", name), Pattern: src, Pos: i + 2}
				}
				g = n
			}
			addGroup(g)
			i = j
		case c == '0':
			v := rune(0)
			for k := 0; k < 2 && i+1 < len(rs) && isOct(rs[i+1]); k++ {
				i++
				v = v*8 + rs[i] - '0'
			}
			lit.WriteRune(v)
		case isDigit(c):
			if i+1 < len(rs) && isDigit(rs[i+1]) {
				if isOct(c) && isOct(rs[i+1]) && i+2 < len(rs) && isOct(rs[i+2]) {
					v := (c-'0')*64 + (rs[i+1]-'0')*8 + rs[i+2] - '0'
					if v > 0o377 {
						return nil, &Error{Msg: "octal escape value outside of range 0-0o377", Pattern: src, Pos: start}
					}
					lit.WriteRune(v)
					i += 2
					continue
				}
				g := int(c-'0')*10 + int(rs[i+1]-'0')
				if g > re.ngroups {
					return nil, &Error{Msg: fmt.Sprintf("invalid group reference %d", g), Pattern: src, Pos: start + 1}
				}
				addGroup(g)
				i++
				continue
			}
			g := int(c - '0')
			if g > re.ngroups {
				return nil, &Error{Msg: fmt.Sprintf("invalid group reference %d", g), Pattern: src, Pos: start + 1}
			}
			addGroup(g)
		default:
			switch c {
			case 'a':
				lit.WriteByte('\a')
			case 'b':
				lit.WriteByte('\b')
			case 'f':
				lit.WriteByte('\f')
			case 'n':
				lit.WriteByte('\n')
			case 'r':
				lit.WriteByte('\r')
			case 't':
				lit.WriteByte('\t')
			case 'v':
				lit.WriteByte('\v')
			case '\\':
				lit.WriteByte('\\')
			default:
				if c < 0x80 && ('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z') {
					return nil, &Error{Msg: fmt.Sprintf("bad escape \\%c", c), Pattern: src, Pos: start}
				}
				lit.WriteByte('\\')
				lit.WriteRune(c)
			}
		}
	}
	flush()
	return t, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if !isDigit(r) {
			return false
		}
	}
	return s != ""
}
