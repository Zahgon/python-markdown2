// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package pyre

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// Regexp is a compiled pattern. Positions are byte offsets into the subject string.
type Regexp struct {
	root     *node
	ngroups  int
	names    map[string]int
	firstLit rune
	// first, when set, accepts every character a match can start with.
	first func(rune) bool
	// anchor restricts where a match can start: at line starts or only at the beginning.
	anchor anchorKind
}

type anchorKind uint8

const (
	anchorNone anchorKind = iota
	anchorLine
	anchorBegin
)

// Match is the result of a successful match.
type Match struct {
	re *Regexp
	// S is the whole subject string.
	S    string
	caps []int
}

type cacheKey struct {
	pattern string
	flags   Flags
}

var cache sync.Map

// Compile compiles a pattern with the given flags; compiled patterns are cached.
func Compile(pattern string, flags Flags) (*Regexp, error) {
	key := cacheKey{pattern, flags}
	if re, ok := cache.Load(key); ok {
		return re.(*Regexp), nil
	}
	root, p, err := parse(pattern, flags)
	if err != nil {
		return nil, err
	}
	if err := checkConds(root, p.ngroups, p); err != nil {
		return nil, err
	}
	re := &Regexp{root: root, ngroups: p.ngroups, names: p.names, firstLit: firstLiteral(root),
		anchor: leadingAnchor(root)}
	if pred, ok := firstChar(root); ok {
		re.first = pred
	}
	cache.Store(key, re)
	return re, nil
}

// MustCompile is Compile for patterns known to be valid; it panics otherwise.
func MustCompile(pattern string, flags Flags) *Regexp {
	re, err := Compile(pattern, flags)
	if err != nil {
		panic("pyre: " + err.Error() + ": " + pattern)
	}
	return re
}

func checkConds(n *node, ngroups int, p *parser) error {
	if n.kind == kCond && n.group > ngroups {
		return p.errorf(0, "invalid group reference %d", n.group)
	}
	for _, s := range n.subs {
		if err := checkConds(s, ngroups, p); err != nil {
			return err
		}
	}
	return nil
}

func firstLiteral(n *node) rune {
	switch n.kind {
	case kLit:
		if !n.icase {
			return n.r
		}
	case kCat:
		if len(n.subs) > 0 {
			return firstLiteral(n.subs[0])
		}
	case kGroup, kAtomic:
		return firstLiteral(n.subs[0])
	case kRepeat:
		if n.min > 0 {
			return firstLiteral(n.subs[0])
		}
	}
	return -1
}

// leadingAnchor reports an anchor every match must start with.
func leadingAnchor(n *node) anchorKind {
	switch n.kind {
	case kBol:
		if n.multiline {
			return anchorLine
		}
		return anchorBegin
	case kBeginText:
		return anchorBegin
	case kCat:
		if len(n.subs) > 0 {
			return leadingAnchor(n.subs[0])
		}
	case kGroup, kAtomic:
		return leadingAnchor(n.subs[0])
	case kAlt:
		best := anchorBegin
		for _, s := range n.subs {
			a := leadingAnchor(s)
			if a == anchorNone {
				return anchorNone
			}
			if a == anchorLine {
				best = anchorLine
			}
		}
		return best
	}
	return anchorNone
}

func isZeroWidth(n *node) bool {
	switch n.kind {
	case kBol, kEol, kBeginText, kEndText, kWordB, kNotWordB, kLookahead, kNegLookahead, kLookbehind, kNegLookbehind:
		return true
	}
	return false
}

// firstChar returns a predicate that accepts every character a match of n can start with; ok is
// false when n can match the empty string or the set is not known.
func firstChar(n *node) (func(rune) bool, bool) {
	switch n.kind {
	case kLit:
		if n.icase {
			lit := n.r
			return func(r rune) bool { return litEqual(lit, r) }, true
		}
		lit := n.r
		return func(r rune) bool { return r == lit }, true
	case kAny:
		if n.dotall {
			return func(rune) bool { return true }, true
		}
		return func(r rune) bool { return r != '\n' }, true
	case kClass:
		cls := n.cls
		return cls.matches, true
	case kGroup, kAtomic:
		return firstChar(n.subs[0])
	case kRepeat:
		if n.min > 0 {
			return firstChar(n.subs[0])
		}
	case kCat:
		for _, s := range n.subs {
			if isZeroWidth(s) {
				continue
			}
			return firstChar(s)
		}
	case kAlt:
		var preds []func(rune) bool
		for _, s := range n.subs {
			p, ok := firstChar(s)
			if !ok {
				return nil, false
			}
			preds = append(preds, p)
		}
		return func(r rune) bool {
			for _, p := range preds {
				if p(r) {
					return true
				}
			}
			return false
		}, true
	}
	return nil, false
}

// NumGroups returns the number of capturing groups.
func (re *Regexp) NumGroups() int { return re.ngroups }

func isCased(r rune) bool { return pyunicode.IsCased(r) }

type machine struct {
	re   *Regexp
	s    string
	end  int
	caps []int
}

func (m *machine) runeAt(i int) (rune, int) {
	r, size := utf8.DecodeRuneInString(m.s[i:m.end])
	return r, size
}

func (m *machine) runeBefore(i int) (rune, int) {
	return utf8.DecodeLastRuneInString(m.s[:i])
}

func isWordRune(r rune, ascii bool) bool {
	if ascii {
		return r < 0x80 && (r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9')
	}
	return pyunicode.IsWord(r)
}

func catMatch(c catKind, r rune, ascii bool) bool {
	switch c {
	case catDigit, catNotDigit:
		var ok bool
		if ascii {
			ok = '0' <= r && r <= '9'
		} else {
			ok = pyunicode.IsDecimal(r)
		}
		return ok == (c == catDigit)
	case catSpace, catNotSpace:
		var ok bool
		if ascii {
			ok = r == ' ' || '\t' <= r && r <= '\r'
		} else {
			ok = pyunicode.IsSpace(r)
		}
		return ok == (c == catSpace)
	}
	return isWordRune(r, ascii) == (c == catWord)
}

func (c *class) raw(r rune) bool {
	for i := 0; i < len(c.ranges); i += 2 {
		if c.ranges[i] <= r && r <= c.ranges[i+1] {
			return true
		}
	}
	for _, k := range c.cats {
		if catMatch(k, r, c.ascii) {
			return true
		}
	}
	return false
}

func (c *class) matches(r rune) bool {
	ok := c.raw(r)
	if !ok && c.icase {
		lo := pyunicode.SimpleLower(r)
		ok = c.raw(lo) || c.raw(pyunicode.SimpleUpper(r))
		for f := unicode.SimpleFold(r); !ok && f != r; f = unicode.SimpleFold(f) {
			ok = c.raw(f)
		}
		for _, f := range pyunicode.IgnoreCaseFixes(lo) {
			if !ok {
				ok = c.raw(f)
			}
		}
	}
	return ok != c.negate
}

func litEqual(pat, r rune) bool {
	if pat == r {
		return true
	}
	lp, lr := pyunicode.SimpleLower(pat), pyunicode.SimpleLower(r)
	if lp == lr {
		return true
	}
	for _, f := range pyunicode.IgnoreCaseFixes(lp) {
		if f == lr {
			return true
		}
	}
	return false
}

// one tests a single-character node at i and returns the position after the character.
func (m *machine) one(n *node, i int) (int, bool) {
	if i >= m.end {
		return 0, false
	}
	r, size := m.runeAt(i)
	switch n.kind {
	case kLit:
		if r == n.r || n.icase && litEqual(n.r, r) {
			return i + size, true
		}
		return 0, false
	case kAny:
		if n.dotall || r != '\n' {
			return i + size, true
		}
		return 0, false
	case kClass:
		if n.cls.matches(r) {
			return i + size, true
		}
		return 0, false
	case kGroup:
		return m.one(n.subs[0], i)
	}
	return 0, false
}

func (m *machine) snapshot() []int { return append([]int(nil), m.caps...) }

func (m *machine) restore(s []int) { copy(m.caps, s) }

func accept(int) bool { return true }

func (m *machine) run(n *node, i int, k func(int) bool) bool {
	switch n.kind {
	case kEmpty:
		return k(i)
	case kLit, kAny, kClass:
		j, ok := m.one(n, i)
		return ok && k(j)
	case kCat:
		return m.seq(n.subs, i, k)
	case kAlt:
		for _, s := range n.subs {
			if m.run(s, i, k) {
				return true
			}
		}
		return false
	case kGroup:
		if n.group == 0 {
			return m.run(n.subs[0], i, k)
		}
		g := 2 * n.group
		return m.run(n.subs[0], i, func(j int) bool {
			s0, s1 := m.caps[g], m.caps[g+1]
			m.caps[g], m.caps[g+1] = i, j
			if k(j) {
				return true
			}
			m.caps[g], m.caps[g+1] = s0, s1
			return false
		})
	case kRepeat:
		return m.repeat(n, i, k)
	case kBol:
		if i == 0 || n.multiline && m.s[i-1] == '\n' {
			return k(i)
		}
		return false
	case kEol:
		if i == m.end || (n.multiline && m.s[i] == '\n') || (!n.multiline && i == m.end-1 && m.s[i] == '\n') {
			return k(i)
		}
		return false
	case kBeginText:
		return i == 0 && k(i)
	case kEndText:
		return i == m.end && k(i)
	case kWordB, kNotWordB:
		if m.end == 0 {
			return false
		}
		before, after := false, false
		if i > 0 {
			r, _ := m.runeBefore(i)
			before = isWordRune(r, n.ascii)
		}
		if i < m.end {
			r, _ := m.runeAt(i)
			after = isWordRune(r, n.ascii)
		}
		if (before != after) == (n.kind == kWordB) {
			return k(i)
		}
		return false
	case kLookahead, kNegLookahead:
		saved := m.snapshot()
		ok := m.run(n.subs[0], i, accept)
		if n.kind == kNegLookahead {
			m.restore(saved)
			return !ok && k(i)
		}
		if ok && k(i) {
			return true
		}
		m.restore(saved)
		return false
	case kLookbehind, kNegLookbehind:
		start := i
		ok := true
		for w := 0; w < n.width; w++ {
			if start == 0 {
				ok = false
				break
			}
			_, size := m.runeBefore(start)
			start -= size
		}
		saved := m.snapshot()
		if ok {
			ok = m.run(n.subs[0], start, func(j int) bool { return j == i })
		}
		if n.kind == kNegLookbehind {
			m.restore(saved)
			return !ok && k(i)
		}
		if ok && k(i) {
			return true
		}
		m.restore(saved)
		return false
	case kBackref:
		g := 2 * n.group
		s0, s1 := m.caps[g], m.caps[g+1]
		if s0 < 0 || s1 < 0 {
			return false
		}
		ref := m.s[s0:s1]
		if !n.icase {
			if i+len(ref) <= m.end && m.s[i:i+len(ref)] == ref {
				return k(i + len(ref))
			}
			return false
		}
		j := i
		for _, pr := range ref {
			if j >= m.end {
				return false
			}
			r, size := m.runeAt(j)
			if !litEqual(pr, r) {
				return false
			}
			j += size
		}
		return k(j)
	case kCond:
		if m.caps[2*n.group+1] >= 0 {
			return m.run(n.subs[0], i, k)
		}
		return m.run(n.subs[1], i, k)
	case kAtomic:
		saved := m.snapshot()
		end := -1
		if m.run(n.subs[0], i, func(j int) bool { end = j; return true }) && k(end) {
			return true
		}
		m.restore(saved)
		return false
	}
	return false
}

func (m *machine) seq(subs []*node, i int, k func(int) bool) bool {
	if len(subs) == 0 {
		return k(i)
	}
	if len(subs) == 1 {
		return m.run(subs[0], i, k)
	}
	first := subs[0]
	if first.single {
		j, ok := m.one(first, i)
		return ok && m.seq(subs[1:], j, k)
	}
	return m.run(first, i, func(j int) bool { return m.seq(subs[1:], j, k) })
}

func (m *machine) repeat(n *node, i int, k func(int) bool) bool {
	sub := n.subs[0]
	if n.possessive {
		saved := m.snapshot()
		greedy := *n
		greedy.possessive = false
		end := -1
		if m.repeat(&greedy, i, func(j int) bool { end = j; return true }) && k(end) {
			return true
		}
		m.restore(saved)
		return false
	}
	if sub.single {
		if !n.lazy {
			count, j := 0, i
			for n.max == maxRepeat || count < n.max {
				nj, ok := m.one(sub, j)
				if !ok {
					break
				}
				j = nj
				count++
			}
			for ; count >= n.min; count-- {
				if k(j) {
					return true
				}
				if count == 0 {
					break
				}
				_, size := m.runeBefore(j)
				j -= size
			}
			return false
		}
		count, j := 0, i
		for count < n.min {
			nj, ok := m.one(sub, j)
			if !ok {
				return false
			}
			j = nj
			count++
		}
		for {
			if k(j) {
				return true
			}
			if n.max != maxRepeat && count >= n.max {
				return false
			}
			nj, ok := m.one(sub, j)
			if !ok {
				return false
			}
			j = nj
			count++
		}
	}
	var try func(count, pos int) bool
	if !n.lazy {
		try = func(count, pos int) bool {
			if count < n.min {
				return m.run(sub, pos, func(j int) bool { return try(count+1, j) })
			}
			if n.max == maxRepeat || count < n.max {
				if m.run(sub, pos, func(j int) bool {
					if j == pos {
						return k(j)
					}
					return try(count+1, j)
				}) {
					return true
				}
			}
			return k(pos)
		}
	} else {
		try = func(count, pos int) bool {
			if count < n.min {
				return m.run(sub, pos, func(j int) bool { return try(count+1, j) })
			}
			if k(pos) {
				return true
			}
			if n.max != maxRepeat && count >= n.max {
				return false
			}
			return m.run(sub, pos, func(j int) bool {
				if j == pos {
					return false
				}
				return try(count+1, j)
			})
		}
	}
	return try(0, i)
}

// exec tries the pattern at each start position in [pos, end] (only at pos when anchored).
// With mustAdvance an empty match at pos is not accepted.
func (re *Regexp) exec(s string, pos, end int, anchored, mustAdvance bool) *Match {
	if end > len(s) {
		end = len(s)
	}
	if pos < 0 {
		pos = 0
	}
	if pos > end {
		return nil
	}
	m := &machine{re: re, s: s, end: end, caps: make([]int, 2*(re.ngroups+1))}
	start := pos
	matchEnd := -1
	accept := func(j int) bool {
		if mustAdvance && start == pos && j == pos {
			return false
		}
		matchEnd = j
		return true
	}
	for {
		if !anchored {
			switch {
			case re.anchor == anchorBegin && start > 0:
				return nil
			case re.anchor == anchorLine && start > 0 && s[start-1] != '\n':
				idx := strings.IndexByte(s[start:end], '\n')
				if idx < 0 {
					return nil
				}
				start += idx + 1
				continue
			case re.firstLit >= 0:
				idx := strings.IndexRune(s[start:end], re.firstLit)
				if idx < 0 {
					return nil
				}
				start += idx
			case re.first != nil:
				for start < end {
					r, size := utf8.DecodeRuneInString(s[start:end])
					if re.first(r) {
						break
					}
					start += size
				}
				if start >= end {
					return nil
				}
			}
		}
		for i := range m.caps {
			m.caps[i] = -1
		}
		if m.run(re.root, start, accept) {
			m.caps[0], m.caps[1] = start, matchEnd
			return &Match{re: re, S: s, caps: m.caps}
		}
		if anchored || start >= end {
			return nil
		}
		_, size := utf8.DecodeRuneInString(s[start:end])
		start += size
	}
}

// MatchAt anchors the pattern at pos (Python's pattern.match(s, pos)).
func (re *Regexp) MatchAt(s string, pos int) *Match { return re.exec(s, pos, len(s), true, false) }

// MatchString is MatchAt(s, 0).
func (re *Regexp) MatchString(s string) *Match { return re.exec(s, 0, len(s), true, false) }

// Search finds the first match at or after pos (Python's pattern.search(s, pos)).
func (re *Regexp) Search(s string, pos int) *Match { return re.exec(s, pos, len(s), false, false) }

// SearchRange is Python's pattern.search(s, pos, endpos).
func (re *Regexp) SearchRange(s string, pos, endpos int) *Match {
	return re.exec(s, pos, endpos, false, false)
}

// FindAll returns every non-overlapping match (Python's finditer).
func (re *Regexp) FindAll(s string) []*Match {
	var out []*Match
	pos, must := 0, false
	for pos <= len(s) {
		m := re.exec(s, pos, len(s), false, must)
		if m == nil {
			break
		}
		out = append(out, m)
		must = m.caps[1] == m.caps[0]
		pos = m.caps[1]
	}
	return out
}

// Sub replaces every match with repl's result (Python's re.sub with a function).
func (re *Regexp) Sub(s string, repl func(*Match) string) string {
	matches := re.FindAll(s)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(s[last:m.caps[0]])
		b.WriteString(repl(m))
		last = m.caps[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// SubTemplate replaces every match with the expansion of a Python replacement template.
func (re *Regexp) SubTemplate(s, template string) (string, error) {
	t, err := re.parseTemplate(template)
	if err != nil {
		return "", err
	}
	return re.Sub(s, t.expand), nil
}

// MustSubTemplate is SubTemplate for templates known to be valid.
func (re *Regexp) MustSubTemplate(s, template string) string {
	out, err := re.SubTemplate(s, template)
	if err != nil {
		panic("pyre: " + err.Error())
	}
	return out
}

// ReplaceLiteral replaces every match with a fixed string.
func (re *Regexp) ReplaceLiteral(s, repl string) string {
	return re.Sub(s, func(*Match) string { return repl })
}

// Split is Python's re.split; groups that did not take part in the match give "".
func (re *Regexp) Split(s string, maxsplit int) []string {
	var out []string
	last, n := 0, 0
	pos, must := 0, false
	for pos <= len(s) {
		if maxsplit > 0 && n >= maxsplit {
			break
		}
		m := re.exec(s, pos, len(s), false, must)
		if m == nil {
			break
		}
		out = append(out, s[last:m.caps[0]])
		for g := 1; g <= re.ngroups; g++ {
			out = append(out, m.Group(g))
		}
		last = m.caps[1]
		n++
		must = m.caps[1] == m.caps[0]
		pos = m.caps[1]
	}
	return append(out, s[last:])
}

// Start returns the start of group g, or -1 when it did not participate.
func (m *Match) Start(g int) int { return m.caps[2*g] }

// End returns the end of group g, or -1 when it did not participate.
func (m *Match) End(g int) int { return m.caps[2*g+1] }

// Group returns the text of group g ("" when it did not participate).
func (m *Match) Group(g int) string {
	if m.caps[2*g] < 0 {
		return ""
	}
	return m.S[m.caps[2*g]:m.caps[2*g+1]]
}

// GroupOK returns the text of group g and whether it participated in the match.
func (m *Match) GroupOK(g int) (string, bool) {
	if m.caps[2*g] < 0 {
		return "", false
	}
	return m.S[m.caps[2*g]:m.caps[2*g+1]], true
}

// Named returns the text of a named group and whether it participated.
func (m *Match) Named(name string) (string, bool) {
	g, ok := m.re.names[name]
	if !ok {
		return "", false
	}
	return m.GroupOK(g)
}

// NumGroups returns the number of capturing groups of the pattern.
func (m *Match) NumGroups() int { return m.re.ngroups }

// Text returns the whole matched text.
func (m *Match) Text() string { return m.S[m.caps[0]:m.caps[1]] }

// Expand is Python's match.expand(template).
func (m *Match) Expand(template string) (string, error) {
	t, err := m.re.parseTemplate(template)
	if err != nil {
		return "", err
	}
	return t.expand(m), nil
}

// Escape is Python's re.escape.
func Escape(s string) string {
	const special = "()[]{}?*+-|^$\\.&~# \t\n\r\v\f"
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
