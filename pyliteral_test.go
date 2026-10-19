// Go port of python-markdown2. Copyright (c) 2007-2008 ActiveState Software Inc. (python-markdown2 test suite)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// The *.opts files of the test cases are Python literals (dicts of keyword arguments for
// markdown2.markdown). pyLiteral reads the subset they use: dicts, lists, tuples, strings (with r
// prefixes), numbers, True, False, None, comments, and the re.compile(...) and lambda forms of
// link patterns.

// compiledPattern is re.compile(pattern, flags) in an opts file.
type compiledPattern struct {
	pattern string
	flags   string
}

// lambdaValue is a lambda in an opts file; source is its text.
type lambdaValue struct{ source string }

type literalParser struct {
	src []rune
	pos int
}

func parsePyLiteral(src string) (any, error) {
	p := &literalParser{src: []rune(src)}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("trailing text at %d", p.pos)
	}
	return v, nil
}

func (p *literalParser) skip() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case unicode.IsSpace(c):
			p.pos++
		case c == '#':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *literalParser) peek() rune {
	p.skip()
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *literalParser) expect(c rune) error {
	if p.peek() != c {
		return fmt.Errorf("expected %q at %d", c, p.pos)
	}
	p.pos++
	return nil
}

func (p *literalParser) ident() string {
	start := p.pos
	for p.pos < len(p.src) && (unicode.IsLetter(p.src[p.pos]) || unicode.IsDigit(p.src[p.pos]) || p.src[p.pos] == '_' || p.src[p.pos] == '.') {
		p.pos++
	}
	return string(p.src[start:p.pos])
}

func (p *literalParser) value() (any, error) {
	switch c := p.peek(); {
	case c == '{':
		p.pos++
		m := map[string]any{}
		for p.peek() != '}' {
			k, err := p.value()
			if err != nil {
				return nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("dict key %v is not a string", k)
			}
			if err := p.expect(':'); err != nil {
				return nil, err
			}
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			m[key] = v
			if p.peek() == ',' {
				p.pos++
			}
		}
		p.pos++
		return m, nil
	case c == '[' || c == '(':
		end := map[rune]rune{'[': ']', '(': ')'}[c]
		p.pos++
		list := []any{}
		for p.peek() != end {
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
			if p.peek() == ',' {
				p.pos++
			}
		}
		p.pos++
		return list, nil
	case c == '"' || c == '\'' || c == 'r' && p.pos+1 < len(p.src) && (p.src[p.pos+1] == '"' || p.src[p.pos+1] == '\''):
		return p.str()
	case c == '-' || unicode.IsDigit(c):
		start := p.pos
		p.pos++
		for p.pos < len(p.src) && unicode.IsDigit(p.src[p.pos]) {
			p.pos++
		}
		return strconv.Atoi(string(p.src[start:p.pos]))
	}
	start := p.pos
	switch name := p.ident(); name {
	case "True":
		return true, nil
	case "False":
		return false, nil
	case "None":
		return nil, nil
	case "re.compile":
		if err := p.expect('('); err != nil {
			return nil, err
		}
		pat, err := p.str()
		if err != nil {
			return nil, err
		}
		flags := ""
		if p.peek() == ',' {
			p.pos++
			p.skip()
			for _, f := range strings.Split(p.ident(), "|") {
				switch strings.TrimSpace(f) {
				case "re.I", "re.IGNORECASE":
					flags += "i"
				}
			}
		}
		if err := p.expect(')'); err != nil {
			return nil, err
		}
		return compiledPattern{pat.(string), flags}, nil
	case "lambda":
		depth := 0
		for p.pos < len(p.src) {
			c := p.src[p.pos]
			if depth == 0 && (c == ',' || c == ')' || c == ']') {
				break
			}
			if c == '(' || c == '[' {
				depth++
			} else if c == ')' || c == ']' {
				depth--
			}
			p.pos++
		}
		return lambdaValue{strings.TrimSpace(string(p.src[start:p.pos]))}, nil
	}
	return nil, fmt.Errorf("unexpected text at %d", start)
}

func (p *literalParser) str() (any, error) {
	p.skip()
	raw := false
	if p.src[p.pos] == 'r' {
		raw = true
		p.pos++
	}
	q := p.src[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.src) && p.src[p.pos] != q {
		c := p.src[p.pos]
		if c == '\\' && p.pos+1 < len(p.src) {
			next := p.src[p.pos+1]
			if raw {
				b.WriteRune(c)
				b.WriteRune(next)
				p.pos += 2
				continue
			}
			switch next {
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			case '\\', '\'', '"':
				b.WriteRune(next)
			default:
				b.WriteRune(c)
				b.WriteRune(next)
			}
			p.pos += 2
			continue
		}
		b.WriteRune(c)
		p.pos++
	}
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unterminated string")
	}
	p.pos++
	return b.String(), nil
}
