// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

// Package pyre is a backtracking regular expression engine with the syntax and matching rules of
// Python's re module (str patterns): lookaround, backreferences, conditionals, atomic groups,
// possessive and lazy repeats, inline and scoped flags, verbose mode, Python's $ and \Z anchors,
// Unicode \w, \d, \s and \b, and Python's case-insensitive matching. markdown2's patterns, and the
// link patterns users give it, are written in this dialect, which Go's regexp package cannot express.
package pyre

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Flags are the re module flags.
type Flags int

// The re module flag values.
const (
	IGNORECASE Flags = 2
	LOCALE     Flags = 4
	MULTILINE  Flags = 8
	DOTALL     Flags = 16
	UNICODE    Flags = 32
	VERBOSE    Flags = 64
	ASCII      Flags = 256
)

// Error is a pattern compilation error (Python's re.error).
type Error struct {
	Msg     string
	Pattern string
	Pos     int
}

func (e *Error) Error() string { return fmt.Sprintf("%s at position %d", e.Msg, e.Pos) }

type kind uint8

const (
	kEmpty kind = iota
	kLit
	kAny
	kClass
	kCat
	kAlt
	kGroup
	kRepeat
	kBol // ^
	kEol // $
	kBeginText
	kEndText
	kWordB
	kNotWordB
	kLookahead
	kNegLookahead
	kLookbehind
	kNegLookbehind
	kBackref
	kCond
	kAtomic
)

const maxRepeat = -1

type node struct {
	kind       kind
	r          rune
	cls        *class
	subs       []*node
	group      int
	min, max   int
	lazy       bool
	possessive bool
	icase      bool
	dotall     bool
	multiline  bool
	ascii      bool
	width      int
	single     bool // matches exactly one character (literal, class or any)
}

type catKind uint8

const (
	catDigit catKind = iota
	catNotDigit
	catSpace
	catNotSpace
	catWord
	catNotWord
)

type class struct {
	negate bool
	ranges []rune // pairs
	cats   []catKind
	icase  bool
	ascii  bool
}

type parser struct {
	src     []rune
	pos     int
	flags   Flags
	ngroups int
	names   map[string]int
	open    map[int]bool
	pattern string
}

func (p *parser) errorf(pos int, format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...), Pattern: p.pattern, Pos: pos}
}

func (p *parser) more() bool { return p.pos < len(p.src) }

func (p *parser) peek() rune {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return -1
}

func (p *parser) peekAt(off int) rune {
	if p.pos+off < len(p.src) {
		return p.src[p.pos+off]
	}
	return -1
}

func parse(pattern string, flags Flags) (*node, *parser, error) {
	p := &parser{src: []rune(pattern), flags: flags, names: map[string]int{}, open: map[int]bool{}, pattern: pattern}
	// Global inline flags are only allowed at the start of the pattern.
	for p.peek() == '(' && p.peekAt(1) == '?' {
		save := p.pos
		p.pos += 2
		var f Flags
		ok := false
		for p.more() {
			c := p.peek()
			if fl, isFlag := flagLetter(c); isFlag {
				f |= fl
				p.pos++
				continue
			}
			ok = c == ')'
			break
		}
		if !ok || p.pos == save+2 {
			p.pos = save
			break
		}
		p.pos++
		if f&LOCALE != 0 {
			return nil, nil, p.errorf(save, "bad inline flags: cannot use 'L' flag with a str pattern")
		}
		p.flags |= f
	}
	if p.flags&ASCII != 0 && p.flags&UNICODE != 0 {
		return nil, nil, p.errorf(0, "ASCII and UNICODE flags are incompatible")
	}
	n, err := p.parseAlt(p.flags)
	if err != nil {
		return nil, nil, err
	}
	if p.more() {
		if p.peek() == ')' {
			return nil, nil, p.errorf(p.pos, "unbalanced parenthesis")
		}
		return nil, nil, p.errorf(p.pos, "unexpected end of pattern")
	}
	return n, p, nil
}

func flagLetter(c rune) (Flags, bool) {
	switch c {
	case 'a':
		return ASCII, true
	case 'i':
		return IGNORECASE, true
	case 'L':
		return LOCALE, true
	case 'm':
		return MULTILINE, true
	case 's':
		return DOTALL, true
	case 'u':
		return UNICODE, true
	case 'x':
		return VERBOSE, true
	}
	return 0, false
}

func (p *parser) parseAlt(flags Flags) (*node, error) {
	var alts []*node
	for {
		seq, err := p.parseSeq(flags)
		if err != nil {
			return nil, err
		}
		alts = append(alts, seq)
		if p.peek() != '|' {
			break
		}
		p.pos++
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return &node{kind: kAlt, subs: alts}, nil
}

func (p *parser) skipVerbose(flags Flags) {
	if flags&VERBOSE == 0 {
		return
	}
	for p.more() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f':
			p.pos++
		case c == '#':
			for p.more() && p.peek() != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *parser) parseSeq(flags Flags) (*node, error) {
	var items []*node
	for {
		p.skipVerbose(flags)
		if !p.more() || p.peek() == '|' || p.peek() == ')' {
			break
		}
		start := p.pos
		atom, err := p.parseAtom(flags)
		if err != nil {
			return nil, err
		}
		if atom == nil { // a comment group or scoped-nothing
			continue
		}
		atom, err = p.parseQuantifier(atom, flags, start)
		if err != nil {
			return nil, err
		}
		items = append(items, atom)
	}
	switch len(items) {
	case 0:
		return &node{kind: kEmpty}, nil
	case 1:
		return items[0], nil
	}
	return &node{kind: kCat, subs: items}, nil
}

func isRepeatable(n *node) bool {
	switch n.kind {
	case kBol, kEol, kBeginText, kEndText, kWordB, kNotWordB:
		return false
	}
	return true
}

func (p *parser) parseQuantifier(atom *node, flags Flags, start int) (*node, error) {
	for {
		p.skipVerbose(flags)
		if !p.more() {
			return atom, nil
		}
		c := p.peek()
		qpos := p.pos
		min, max := 0, 0
		switch c {
		case '*':
			min, max = 0, maxRepeat
			p.pos++
		case '+':
			min, max = 1, maxRepeat
			p.pos++
		case '?':
			min, max = 0, 1
			p.pos++
		case '{':
			lo, hi, ok := p.parseBraces()
			if !ok {
				return atom, nil
			}
			min, max = lo, hi
			if max != maxRepeat && max < min {
				return nil, p.errorf(qpos+1, "min repeat greater than max repeat")
			}
		default:
			return atom, nil
		}
		if !isRepeatable(atom) || atom.kind == kEmpty {
			return nil, p.errorf(qpos, "nothing to repeat")
		}
		if atom.kind == kRepeat {
			return nil, p.errorf(qpos, "multiple repeat")
		}
		rep := &node{kind: kRepeat, subs: []*node{atom}, min: min, max: max, single: false}
		if p.peek() == '?' {
			rep.lazy = true
			p.pos++
		} else if p.peek() == '+' {
			rep.possessive = true
			p.pos++
		}
		atom = rep
	}
}

func (p *parser) parseBraces() (int, int, bool) {
	save := p.pos
	p.pos++
	readInt := func() (string, bool) {
		s := p.pos
		for p.more() && p.peek() >= '0' && p.peek() <= '9' {
			p.pos++
		}
		return string(p.src[s:p.pos]), p.pos > s
	}
	loS, hasLo := readInt()
	lo, hi := 0, maxRepeat
	if p.peek() == ',' {
		p.pos++
		hiS, hasHi := readInt()
		if hasHi {
			hi = atoiSat(hiS)
		}
	} else {
		if !hasLo {
			p.pos = save
			return 0, 0, false
		}
		hi = atoiSat(loS)
	}
	if p.peek() != '}' {
		p.pos = save
		return 0, 0, false
	}
	p.pos++
	if hasLo {
		lo = atoiSat(loS)
	}
	return lo, hi, true
}

func atoiSat(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n > 1<<30 {
		return 1 << 30
	}
	return n
}

func (p *parser) parseAtom(flags Flags) (*node, error) {
	c := p.peek()
	start := p.pos
	p.pos++
	ic := flags&IGNORECASE != 0
	switch c {
	case '.':
		return &node{kind: kAny, dotall: flags&DOTALL != 0, single: true}, nil
	case '^':
		return &node{kind: kBol, multiline: flags&MULTILINE != 0}, nil
	case '$':
		return &node{kind: kEol, multiline: flags&MULTILINE != 0}, nil
	case '[':
		cls, err := p.parseClass(flags)
		if err != nil {
			return nil, err
		}
		return &node{kind: kClass, cls: cls, single: true}, nil
	case '(':
		return p.parseGroup(flags, start)
	case '\\':
		return p.parseEscape(flags, start)
	case '*', '+', '?':
		return nil, p.errorf(start, "nothing to repeat")
	case '{':
		save := p.pos
		p.pos = start
		if _, _, ok := p.parseBraces(); ok {
			return nil, p.errorf(start, "nothing to repeat")
		}
		p.pos = save
		return lit('{', ic), nil
	}
	return lit(c, ic), nil
}

func lit(r rune, icase bool) *node {
	return &node{kind: kLit, r: r, icase: icase && isCased(r), single: true}
}

func (p *parser) parseGroupName(term rune) (string, error) {
	start := p.pos
	for p.more() && p.peek() != term {
		p.pos++
	}
	if !p.more() {
		return "", p.errorf(start, "missing %c, unterminated name", term)
	}
	name := string(p.src[start:p.pos])
	p.pos++
	if name == "" {
		return "", p.errorf(start, "missing group name")
	}
	return name, nil
}

func isIdentifier(s string) bool {
	for i, r := range s {
		if r == '_' || unicode.IsLetter(r) || i > 0 && (unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r)) {
			continue
		}
		return false
	}
	return s != ""
}

func (p *parser) parseGroup(flags Flags, start int) (*node, error) {
	capture := true
	var name string
	if p.peek() == '?' {
		p.pos++
		c := p.peek()
		p.pos++
		switch c {
		case 'P':
			switch p.peek() {
			case '<':
				p.pos++
				nm, err := p.parseGroupName('>')
				if err != nil {
					return nil, err
				}
				if !isIdentifier(nm) {
					return nil, p.errorf(start+4, "bad character in group name %s", strconv.Quote(nm))
				}
				name = nm
			case '=':
				p.pos++
				nm, err := p.parseGroupName(')')
				if err != nil {
					return nil, err
				}
				g, ok := p.names[nm]
				if !ok {
					return nil, p.errorf(start+4, "unknown group name '%s'", nm)
				}
				if p.open[g] {
					return nil, p.errorf(start+4, "cannot refer to an open group")
				}
				return &node{kind: kBackref, group: g, icase: flags&IGNORECASE != 0}, nil
			default:
				if !p.more() {
					return nil, p.errorf(p.pos, "unexpected end of pattern")
				}
				return nil, p.errorf(p.pos, "unknown extension ?P%c", p.peek())
			}
		case ':':
			capture = false
		case '#':
			for p.more() && p.peek() != ')' {
				p.pos++
			}
			if !p.more() {
				return nil, p.errorf(start, "missing ), unterminated comment")
			}
			p.pos++
			return nil, nil
		case '=', '!':
			sub, err := p.parseAlt(flags)
			if err != nil {
				return nil, err
			}
			if err := p.close(start); err != nil {
				return nil, err
			}
			k := kLookahead
			if c == '!' {
				k = kNegLookahead
			}
			return &node{kind: k, subs: []*node{sub}}, nil
		case '<':
			d := p.peek()
			if d != '=' && d != '!' {
				if !p.more() {
					return nil, p.errorf(p.pos, "unexpected end of pattern")
				}
				return nil, p.errorf(p.pos, "unknown extension ?<%c", d)
			}
			p.pos++
			sub, err := p.parseAlt(flags)
			if err != nil {
				return nil, err
			}
			if err := p.close(start); err != nil {
				return nil, err
			}
			lo, hi := width(sub)
			if lo != hi {
				return nil, p.errorf(start, "look-behind requires fixed-width pattern")
			}
			k := kLookbehind
			if d == '!' {
				k = kNegLookbehind
			}
			return &node{kind: k, subs: []*node{sub}, width: lo}, nil
		case '>':
			sub, err := p.parseAlt(flags)
			if err != nil {
				return nil, err
			}
			if err := p.close(start); err != nil {
				return nil, err
			}
			return &node{kind: kAtomic, subs: []*node{sub}}, nil
		case '(':
			cstart := p.pos
			for p.more() && p.peek() != ')' {
				p.pos++
			}
			if !p.more() {
				return nil, p.errorf(cstart, "missing ), unterminated name")
			}
			ref := string(p.src[cstart:p.pos])
			p.pos++
			var g int
			if n, err := strconv.Atoi(ref); err == nil && ref[0] >= '0' && ref[0] <= '9' {
				if n == 0 {
					return nil, p.errorf(cstart, "bad group number")
				}
				g = n
			} else if gi, ok := p.names[ref]; ok {
				g = gi
			} else {
				if !isIdentifier(ref) {
					return nil, p.errorf(cstart, "bad character in group name %s", strconv.Quote(ref))
				}
				return nil, p.errorf(cstart, "unknown group name '%s'", ref)
			}
			yes, err := p.parseSeq(flags)
			if err != nil {
				return nil, err
			}
			no := &node{kind: kEmpty}
			if p.peek() == '|' {
				p.pos++
				no, err = p.parseSeq(flags)
				if err != nil {
					return nil, err
				}
				if p.peek() == '|' {
					return nil, p.errorf(p.pos, "conditional backref with more than two branches")
				}
			}
			if err := p.close(start); err != nil {
				return nil, err
			}
			if g >= 100 {
				return nil, p.errorf(cstart, "invalid group reference %d", g)
			}
			return &node{kind: kCond, group: g, subs: []*node{yes, no}}, nil
		default:
			// scoped or global flags
			p.pos--
			on, off := Flags(0), Flags(0)
			fstart := p.pos
			for p.more() {
				f, ok := flagLetter(p.peek())
				if !ok {
					break
				}
				on |= f
				p.pos++
			}
			if p.peek() == '-' {
				p.pos++
				for p.more() {
					f, ok := flagLetter(p.peek())
					if !ok {
						break
					}
					off |= f
					p.pos++
				}
			}
			switch p.peek() {
			case ':':
				p.pos++
				if on&(LOCALE) != 0 {
					return nil, p.errorf(fstart, "bad inline flags: cannot use 'L' flag with a str pattern")
				}
				nf := (flags | on) &^ off
				if on&ASCII != 0 {
					nf &^= UNICODE
				}
				sub, err := p.parseAlt(nf)
				if err != nil {
					return nil, err
				}
				if err := p.close(start); err != nil {
					return nil, err
				}
				return &node{kind: kGroup, subs: []*node{sub}, single: sub.single}, nil
			case ')':
				if p.pos == fstart {
					return nil, p.errorf(fstart, "unknown extension ?)")
				}
				return nil, p.errorf(start, "global flags not at the start of the expression")
			}
			if !p.more() {
				return nil, p.errorf(p.pos, "missing -, : or )")
			}
			if p.pos == fstart {
				return nil, p.errorf(fstart, "unknown extension ?%c", p.peek())
			}
			return nil, p.errorf(p.pos, "missing -, : or )")
		}
	}
	idx := 0
	if capture {
		p.ngroups++
		idx = p.ngroups
		if name != "" {
			if _, dup := p.names[name]; dup {
				return nil, p.errorf(start+4, "redefinition of group name '%s' as group %d; was group %d", name, idx, p.names[name])
			}
			p.names[name] = idx
		}
		p.open[idx] = true
	}
	sub, err := p.parseAlt(flags)
	if err != nil {
		return nil, err
	}
	if err := p.close(start); err != nil {
		return nil, err
	}
	if !capture {
		return &node{kind: kGroup, group: 0, subs: []*node{sub}, single: sub.single}, nil
	}
	delete(p.open, idx)
	return &node{kind: kGroup, group: idx, subs: []*node{sub}}, nil
}

func (p *parser) close(start int) error {
	if p.peek() != ')' {
		return p.errorf(start, "missing ), unterminated subpattern")
	}
	p.pos++
	return nil
}

// width returns the minimum and maximum number of characters n can match (max -1 = unbounded).
func width(n *node) (int, int) {
	switch n.kind {
	case kLit, kAny, kClass:
		return 1, 1
	case kCat:
		lo, hi := 0, 0
		for _, s := range n.subs {
			a, b := width(s)
			lo += a
			if hi >= 0 && b >= 0 {
				hi += b
			} else {
				hi = -1
			}
		}
		return lo, hi
	case kAlt:
		lo, hi := -1, 0
		for _, s := range n.subs {
			a, b := width(s)
			if lo < 0 || a < lo {
				lo = a
			}
			if hi >= 0 && (b < 0 || b > hi) {
				hi = b
			}
		}
		return lo, hi
	case kGroup, kAtomic:
		return width(n.subs[0])
	case kCond:
		a1, b1 := width(n.subs[0])
		a2, b2 := width(n.subs[1])
		lo, hi := a1, b1
		if a2 < lo {
			lo = a2
		}
		if hi >= 0 && (b2 < 0 || b2 > hi) {
			hi = b2
		}
		return lo, hi
	case kRepeat:
		a, b := width(n.subs[0])
		lo := a * n.min
		if n.max == maxRepeat || b < 0 {
			if b == 0 {
				return lo, 0
			}
			return lo, -1
		}
		return lo, b * n.max
	case kBackref:
		return 0, -1
	}
	return 0, 0
}

func (p *parser) parseEscape(flags Flags, start int) (*node, error) {
	if !p.more() {
		return nil, p.errorf(start, "bad escape (end of pattern)")
	}
	c := p.peek()
	p.pos++
	ic := flags&IGNORECASE != 0
	ascii := flags&ASCII != 0
	switch c {
	case 'A':
		return &node{kind: kBeginText}, nil
	case 'Z':
		return &node{kind: kEndText}, nil
	case 'b':
		return &node{kind: kWordB, ascii: ascii}, nil
	case 'B':
		return &node{kind: kNotWordB, ascii: ascii}, nil
	case 'd', 'D', 's', 'S', 'w', 'W':
		return &node{kind: kClass, cls: &class{cats: []catKind{catOf(c)}, ascii: ascii}, single: true}, nil
	}
	if c >= '1' && c <= '9' {
		// octal escape (three octal digits) or group reference
		if c <= '7' && p.peek() >= '0' && p.peek() <= '7' && p.peekAt(1) >= '0' && p.peekAt(1) <= '7' {
			v := (c-'0')*64 + (p.peek()-'0')*8 + (p.peekAt(1) - '0')
			p.pos += 2
			if v > 0o377 {
				return nil, p.errorf(start, "octal escape value \\%s outside of range 0-0o377", string(p.src[start+1:p.pos]))
			}
			return lit(v, ic), nil
		}
		g := int(c - '0')
		if p.peek() >= '0' && p.peek() <= '9' {
			g = g*10 + int(p.peek()-'0')
			p.pos++
		}
		if g > p.ngroups {
			return nil, p.errorf(start, "invalid group reference %d", g)
		}
		if p.open[g] {
			return nil, p.errorf(start, "cannot refer to an open group")
		}
		return &node{kind: kBackref, group: g, icase: ic}, nil
	}
	r, err := p.charEscape(c, start, false)
	if err != nil {
		return nil, err
	}
	return lit(r, ic), nil
}

func catOf(c rune) catKind {
	switch c {
	case 'd':
		return catDigit
	case 'D':
		return catNotDigit
	case 's':
		return catSpace
	case 'S':
		return catNotSpace
	case 'w':
		return catWord
	}
	return catNotWord
}

// charEscape decodes an escape that stands for one character (c follows the backslash).
func (p *parser) charEscape(c rune, start int, inClass bool) (rune, error) {
	switch c {
	case 'a':
		return '\a', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'v':
		return '\v', nil
	case '\\':
		return '\\', nil
	case 'x', 'u', 'U':
		n := map[rune]int{'x': 2, 'u': 4, 'U': 8}[c]
		if p.pos+n > len(p.src) {
			return 0, p.errorf(start, "incomplete escape \\%s", string(p.src[start+1:]))
		}
		digits := string(p.src[p.pos : p.pos+n])
		v, err := strconv.ParseUint(digits, 16, 32)
		if err != nil || strings.ContainsAny(digits, "+-") {
			end := p.pos
			for end < len(p.src) && end < p.pos+n && isHexRune(p.src[end]) {
				end++
			}
			return 0, p.errorf(start, "incomplete escape \\%s", string(p.src[start+1:end]))
		}
		p.pos += n
		if v > 0x10ffff {
			return 0, p.errorf(start, "bad escape \\%s", string(p.src[start+1:p.pos]))
		}
		return rune(v), nil
	case '0':
		v := rune(0)
		for i := 0; i < 2 && p.peek() >= '0' && p.peek() <= '7'; i++ {
			v = v*8 + p.peek() - '0'
			p.pos++
		}
		return v, nil
	case 'N':
		return 0, p.errorf(start, "bad escape \\N")
	}
	if inClass && c >= '1' && c <= '7' {
		v := c - '0'
		for i := 0; i < 2 && p.peek() >= '0' && p.peek() <= '7'; i++ {
			v = v*8 + p.peek() - '0'
			p.pos++
		}
		if v > 0o377 {
			return 0, p.errorf(start, "octal escape value \\%s outside of range 0-0o377", string(p.src[start+1:p.pos]))
		}
		return v, nil
	}
	if c < 0x80 && (unicode.IsLetter(c) || unicode.IsDigit(c)) {
		return 0, p.errorf(start, "bad escape \\%c", c)
	}
	return c, nil
}

func isHexRune(r rune) bool {
	return '0' <= r && r <= '9' || 'a' <= r && r <= 'f' || 'A' <= r && r <= 'F'
}

func (p *parser) parseClass(flags Flags) (*class, error) {
	start := p.pos - 1
	cls := &class{icase: flags&IGNORECASE != 0, ascii: flags&ASCII != 0}
	if p.peek() == '^' {
		cls.negate = true
		p.pos++
	}
	first := true
	for {
		if !p.more() {
			return nil, p.errorf(start, "unterminated character set")
		}
		c := p.peek()
		if c == ']' && !first {
			p.pos++
			break
		}
		first = false
		itemStart := p.pos
		p.pos++
		var lo rune
		isCat := false
		if c == '\\' {
			if !p.more() {
				return nil, p.errorf(itemStart, "bad escape (end of pattern)")
			}
			e := p.peek()
			p.pos++
			switch e {
			case 'd', 'D', 's', 'S', 'w', 'W':
				cls.cats = append(cls.cats, catOf(e))
				isCat = true
			case 'b':
				lo = '\b'
			default:
				if e >= '8' && e <= '9' {
					return nil, p.errorf(itemStart, "bad escape \\%c", e)
				}
				r, err := p.charEscape(e, itemStart, true)
				if err != nil {
					return nil, err
				}
				lo = r
			}
		} else {
			lo = c
		}
		if p.peek() == '-' && p.peekAt(1) != ']' && p.peekAt(1) != -1 {
			p.pos++
			hiStart := p.pos
			hc := p.peek()
			p.pos++
			var hi rune
			if hc == '\\' {
				if !p.more() {
					return nil, p.errorf(hiStart, "bad escape (end of pattern)")
				}
				e := p.peek()
				p.pos++
				switch e {
				case 'd', 'D', 's', 'S', 'w', 'W':
					return nil, p.errorf(itemStart, "bad character range %s", string(p.src[itemStart:p.pos]))
				case 'b':
					hi = '\b'
				default:
					if e >= '8' && e <= '9' {
						return nil, p.errorf(hiStart, "bad escape \\%c", e)
					}
					r, err := p.charEscape(e, hiStart, true)
					if err != nil {
						return nil, err
					}
					hi = r
				}
			} else {
				hi = hc
			}
			if isCat {
				return nil, p.errorf(itemStart, "bad character range %s", string(p.src[itemStart:p.pos]))
			}
			if hi < lo {
				return nil, p.errorf(itemStart, "bad character range %s", string(p.src[itemStart:p.pos]))
			}
			cls.ranges = append(cls.ranges, lo, hi)
			continue
		}
		if !isCat {
			cls.ranges = append(cls.ranges, lo, lo)
		}
	}
	return cls, nil
}
