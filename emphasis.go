// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"strings"
	"unicode/utf8"

	"github.com/trentm/python-markdown2/internal/pyre"
)

// gfmEmphasis turns * and _ delimiter runs into <em> and <strong>, close to GitHub Flavoured
// Markdown: runs are paired by flanking rules, unequal runs are split, and spans may not cross
// the borders of inline HTML elements.
type gfmEmphasis struct {
	c *Converter
	// spanHook replaces the default span rendering (the code-friendly extra); ok=false falls back.
	spanHook func(open, close, middle *pyre.Match, openSyntax, closeSyntax string) (tokens []string, used int, hasUsed bool, ok bool)
}

type delimRun struct {
	m           *pyre.Match
	left, right bool
}

var delimRunRe = rx(`(\*+|_+)`, 0)

func (g *gfmEmphasis) test(text string) bool {
	return strings.Count(text, "*") > 1 || strings.Count(text, "_") > 1
}

// syntaxAfter returns run's delimiter text after the first offset characters ("" when used up).
func syntaxAfter(m *pyre.Match, offset int) string {
	s := m.Group(1)
	if offset >= len(s) {
		return ""
	}
	return s[offset:]
}

func (g *gfmEmphasis) run(text string) string {
	nesting := true
	orig := ""
	for nesting && orig != hashText(text) {
		orig = hashText(text)
		nesting = false

		opens := map[byte][]*pyre.Match{'*': nil, '_': nil}
		unusedOpens := map[byte]map[*pyre.Match]int{'*': {}, '_': {}}
		unusedCloses := map[byte][]*pyre.Match{'*': nil, '_': nil}
		var tokens []string
		index := 0

		runs := delimRunRe.FindAll(text)
		ri := 0
		nextRun := func() *delimRun {
			for ri < len(runs) {
				m := runs[ri]
				ri++
				left, right := g.leftOrRight(m)
				if left || right {
					return &delimRun{m, left, right}
				}
			}
			return nil
		}
		next := nextRun()
		for next != nil {
			cur := next
			delim, left, right := cur.m, cur.left, cur.right
			next = nextRun()

			syntax := delim.Group(1)
			em := syntax[0]
			if !right || len(opens[em]) == 0 {
				if left {
					opens[em] = append(opens[em], delim)
				}
				continue
			}
			for len(opens[em]) > 0 && g.bodyCrossesSpanBorders(opens[em][len(opens[em])-1], delim) {
				opens[em] = opens[em][:len(opens[em])-1]
			}
			if len(opens[em]) == 0 {
				if left {
					opens[em] = append(opens[em], delim)
				}
				continue
			}
			open := opens[em][len(opens[em])-1]
			opens[em] = opens[em][:len(opens[em])-1]

			if open.Start(0) < index {
				nesting = true
				continue
			}
			openOffset := popOffset(unusedOpens[em], open)
			openSyntax := syntaxAfter(open, openOffset)

			var middle *pyre.Match
			if openSyntax != syntax {
				newOpen, mid, has := g.hasMiddle(open, delim, &opens, em, unusedOpens[em], unusedCloses[em])
				if has {
					middle = mid
					if newOpen != open {
						open = newOpen
						openOffset = popOffset(unusedOpens[em], open)
						openSyntax = syntaxAfter(open, openOffset)
					}
				} else if !g.shouldProcessImbalanced(open, delim, unusedOpens[em], next) {
					unusedOpens[em][open] = openOffset
					opens[em] = append(opens[em], open)
					if left {
						unusedOpens[em][delim] = 0
						opens[em] = append(opens[em], delim)
					} else {
						unusedCloses[em] = append(unusedCloses[em], delim)
					}
					continue
				}
			}

			tokens = append(tokens, slice(text, index, open.Start(0)+openOffset))
			span, used, hasUsed := g.processSpan(open, delim, middle, openSyntax, syntax)
			tokens = append(tokens, span...)

			if !hasUsed {
				used = len(syntax)
			} else if used < len(syntax) {
				unusedOpens[em][delim] = used
				opens[em] = append(opens[em], delim)
			} else if used < len(openSyntax) && len(opens[em]) > 0 {
				prev := opens[em][len(opens[em])-1]
				if len(prev.Group(1)) >= len(openSyntax)-used {
					nesting = true
				}
			}
			index = delim.Start(0) + used
		}
		if index < len(text) {
			tokens = append(tokens, text[index:])
		}
		text = strings.Join(tokens, "")
	}
	return text
}

func popOffset(m map[*pyre.Match]int, k *pyre.Match) int {
	v := m[k]
	delete(m, k)
	return v
}

// orSyntax is Python's "s or fallback".
func orSyntax(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func minStr(a, b string) string {
	if a < b {
		return a
	}
	return b
}

func maxStr(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func cutTail(s string, n int) string {
	if n <= 0 || n >= len(s) {
		return ""
	}
	return s[:len(s)-n]
}

func (g *gfmEmphasis) processSpan(open, close, middle *pyre.Match, openSyntax, closeSyntax string) ([]string, int, bool) {
	if g.spanHook != nil {
		if tokens, used, hasUsed, ok := g.spanHook(open, close, middle, openSyntax, closeSyntax); ok {
			return tokens, used, hasUsed
		}
	}
	openSyntax = orSyntax(openSyntax, open.Group(1))
	closeSyntax = orSyntax(closeSyntax, close.Group(1))
	middleSyntax := ""
	if middle != nil {
		middleSyntax = middle.Group(1)
	}
	outer := len(minStr(openSyntax, closeSyntax))
	inner := 0
	if middle != nil {
		inner = len(minStr(maxStr(openSyntax, closeSyntax), middleSyntax))
	}
	tokens := []string{
		cutTail(openSyntax, outer+inner),
		strings.Repeat("<em>", outer%2),
		strings.Repeat("<strong>", outer/2),
	}
	s := close.S
	if middle != nil {
		tag := "em"
		if len(middleSyntax) == 2 {
			tag = "strong"
		}
		if openSyntax > closeSyntax {
			tokens = append(tokens, "<"+tag+">")
		}
		tokens = append(tokens, slice(s, open.End(0), middle.Start(0)))
		if openSyntax > closeSyntax {
			tokens = append(tokens, "</"+tag+">")
		} else {
			tokens = append(tokens, "<"+tag+">")
		}
		tokens = append(tokens, slice(s, middle.End(0), close.Start(0)))
		if openSyntax < closeSyntax {
			tokens = append(tokens, "</"+tag+">")
		}
	} else {
		tokens = append(tokens, slice(s, open.End(0), close.Start(0)))
	}
	tokens = append(tokens, strings.Repeat("</strong>", outer/2)+strings.Repeat("</em>", outer%2))
	used := outer
	if middle != nil && openSyntax < closeSyntax {
		used += inner
	}
	return tokens, used, true
}

// slice is Python's s[a:b] for non-negative bounds.
func slice(s string, a, b int) string {
	if a > len(s) {
		a = len(s)
	}
	if b > len(s) {
		b = len(s)
	}
	if a >= b {
		return ""
	}
	return s[a:b]
}

func (g *gfmEmphasis) hasMiddle(open, close *pyre.Match, opens *map[byte][]*pyre.Match, em byte,
	unusedOpens map[*pyre.Match]int, unusedCloses []*pyre.Match) (*pyre.Match, *pyre.Match, bool) {
	openSyntax := syntaxAfter(open, unusedOpens[open])
	syntax := close.Group(1)
	list := (*opens)[em]
	var middle *pyre.Match
	switch {
	case openSyntax < syntax && len(list) > 0:
		if g.bodyCrossesSpanBorders(list[len(list)-1], open) {
			return nil, nil, false
		}
		middle = open
		open = list[len(list)-1]
		(*opens)[em] = list[:len(list)-1]
		offset := popOffset(unusedOpens, open)
		if syntaxAfter(open, offset) == syntax {
			middle = nil
		}
	case openSyntax > syntax && len(unusedCloses) > 0:
		for _, c := range unusedCloses {
			if open.End(0) < c.Start(0) && c.Start(0) < close.Start(0) {
				middle = c
				break
			}
		}
	default:
		return nil, nil, false
	}
	return open, middle, true
}

func (g *gfmEmphasis) shouldProcessImbalanced(open, close *pyre.Match, unusedOpens map[*pyre.Match]int, next *delimRun) bool {
	if next == nil {
		return true
	}
	openSyntax := syntaxAfter(open, unusedOpens[open])
	syntax := close.Group(1)
	if openSyntax < syntax && len(syntax) >= 3 {
		return true
	}
	nextSyntax := next.m.Group(1)
	if nextSyntax[0] != syntax[0] {
		return true
	}
	left, right := g.leftOrRight(close)
	if openSyntax < syntax && (left && (!next.right || nextSyntax < openSyntax+syntax)) && !next.left {
		return true
	}
	if openSyntax > syntax && (right && !next.left) && !next.right {
		return true
	}
	return false
}

// flankKey identifies a delimiter run by its text with one character of context on each side.
type flankKey struct{ run, syntax string }

// leftOrRight reports whether a delimiter run is left- and right-flanking.
func (g *gfmEmphasis) leftOrRight(m *pyre.Match) (bool, bool) {
	s := m.S
	start, end := m.Start(0), m.End(0)
	if start > 0 {
		_, size := utf8.DecodeLastRuneInString(s[:start])
		start -= size
	}
	if end < len(s) {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
	}
	run := s[start:end]
	key := flankKey{run, m.Group(1)}
	if r, ok := g.c.flankCache[key]; ok {
		return r[0], r[1]
	}
	syn := strings.ReplaceAll(m.Group(1), "*", `\*`)
	left := mustCompile(`.*`+syn+`\S`, reS).MatchString(run) != nil &&
		(mustCompile(`.*`+syn+`[\s\w]`, reS).MatchString(run) != nil ||
			mustCompile(`(^|[\s\W])`+syn+`([^\s\w]|$)`, reS|reM).MatchString(run) != nil)
	right := mustCompile(`\S`+syn+`.*`, reS).MatchString(run) != nil &&
		(mustCompile(`[\s\w]`+syn+`.*`, reS).MatchString(run) != nil ||
			mustCompile(`[^\s\w]`+syn+`(\s|[^\s\w]|$)`, reS|reM).MatchString(run) != nil)
	if len(g.c.flankCache) >= 4096 {
		g.c.flankCache = map[flankKey][2]bool{}
	}
	g.c.flankCache[key] = [2]bool{left, right}
	return left, right
}

var spanTagRe = rx(`</?(`+spanTags+`)`, 0)

func (g *gfmEmphasis) bodyCrossesSpanBorders(open, close *pyre.Match) bool {
	text := slice(open.S, open.End(0), close.Start(0))
	if len(text) < 7 || len(text) < 4*7 && utf8.RuneCountInString(text) < 7 {
		return false
	}
	seen := map[string]bool{}
	for _, m := range spanTagRe.FindAll(text) {
		tag := m.Group(1)
		if seen[tag] {
			continue
		}
		seen[tag] = true
		if !g.c.tagIsClosed(tag, text) {
			return true
		}
	}
	return false
}
