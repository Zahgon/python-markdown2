// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"strconv"
	"strings"

	"github.com/trentm/python-markdown2/internal/pyre"
	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// extra is an optional syntax feature that runs before or after conversion stages.
type extra interface {
	test(text string) bool
	run(text string) string
}

type registration struct {
	name string
	make func(c *Converter, options any) extra
}

// extraRegistry lists the extras in registration order.
var extraRegistry = []registration{
	{"admonitions", func(c *Converter, o any) extra { return &admonitions{c} }},
	{"alerts", func(c *Converter, o any) extra { return &alerts{} }},
	{"breaks", func(c *Converter, o any) extra { return &breaks{c, orDict(o)} }},
	{"code-friendly", newCodeFriendly},
	{"emojis", newEmojis},
	{"fenced-code-blocks", func(c *Converter, o any) extra { return &fencedCodeBlocks{c: c} }},
	{"latex", func(c *Converter, o any) extra { return &latex{} }},
	{"link-patterns", func(c *Converter, o any) extra { return &linkPatterns{c, o} }},
	{"markdown-in-html", func(c *Converter, o any) extra { return &markdownInHTML{c} }},
	{"markdown-file-links", newMarkdownFileLinks},
	{"middle-word-em", newMiddleWordEm},
	{"mermaid", func(c *Converter, o any) extra { return &fencedCodeBlocks{c: c, mermaid: true} }},
	{"numbering", func(c *Converter, o any) extra { return &numbering{c} }},
	{"pyshell", func(c *Converter, o any) extra { return &pyshell{c} }},
	{"smarty-pants", func(c *Converter, o any) extra { return &smartyPants{c} }},
	{"strike", func(c *Converter, o any) extra { return &templateExtra{strikeRe, `<s>\1</s>`, "~~"} }},
	{"tables", func(c *Converter, o any) extra { return &tables{c} }},
	{"tg-spoiler", func(c *Converter, o any) extra {
		return &templateExtra{tgSpoilerRe, `<tg-spoiler>\1</tg-spoiler>`, "||"}
	}},
	{"underline", func(c *Converter, o any) extra { return &templateExtra{underlineRe, `<u>\1</u>`, "--"} }},
	{"wavedrom", func(c *Converter, o any) extra { return &wavedrom{c, orDict(o)} }},
	{"wiki-links", func(c *Converter, o any) extra { return &wikiLinks{c, orDict(o)} }},
	{"wiki-tables", func(c *Converter, o any) extra { return &wikiTables{c} }},
}

// orDict gives an extra the options it was enabled with, or an empty dict for none.
func orDict(o any) any {
	if o == nil {
		return map[string]any{}
	}
	return o
}

type stageExtras struct{ before, after []string }

// execOrder lists the extras that run before and after each stage, in order.
var execOrder = map[Stage]stageExtras{
	StagePreprocess:  {nil, []string{"latex", "mermaid", "wavedrom", "fenced-code-blocks"}},
	StageHashHTML:    {nil, []string{"markdown-in-html"}},
	StageLinkDefs:    {[]string{"numbering", "latex", "mermaid", "wavedrom", "fenced-code-blocks", "admonitions"}, []string{"markdown-file-links"}},
	StageBlockGamut:  {[]string{"latex", "mermaid", "wavedrom", "fenced-code-blocks", "admonitions"}, nil},
	StageLists:       {nil, []string{"pyshell", "wiki-tables", "tables"}},
	StageCodeBlocks:  {[]string{"wavedrom", "latex"}, nil},
	StageBlockQuotes: {nil, []string{"alerts"}},
	StageParagraphs:  {nil, []string{"emojis"}},
	StageSpanGamut:   {nil, []string{"smarty-pants"}},
	StageLinks:       {[]string{"wiki-links", "markdown-file-links", "link-patterns"}, nil},
	StageItalicAndBold: {[]string{"underline", "strike", "middle-word-em", "code-friendly"},
		[]string{"breaks", "middle-word-em", "code-friendly", "tg-spoiler"}},
}

// ---- admonitions

const admonitionNames = `admonition|attention|caution|danger|error|hint|important|note|tip|warning`

var admonitionsRe = rx(`
        ^(\ *)\.\.\ (`+admonitionNames+`)::\ *                # $1 leading indent, $2 the admonition
        (.*)?                                # $3 admonition title
        ((?:\s*\n\1\ {3,}.*)+?)              # $4 admonition body (required)
        (?=\s*(?:\Z|\n{4,}|\n\1?\ {0,2}\S))  # until EOF, 3 blank lines or something less indented
        `, reI|reM|reX)

type admonitions struct{ c *Converter }

func (a *admonitions) test(text string) bool { return admonitionsRe.Search(text, 0) != nil }

func (a *admonitions) run(text string) string {
	return admonitionsRe.Sub(text, func(m *pyre.Match) string {
		leadIndent, name, body := m.Group(1), m.Group(2), m.Group(4)
		title, hasTitle := m.GroupOK(3)
		typ := "<strong>" + name + "</strong>"
		class := "admonition"
		if pyunicode.Lower(name) != "admonition" {
			class = "admonition " + pyunicode.Lower(name)
		}
		if title != "" {
			title = "<em>" + title + "</em>"
		} else if !hasTitle {
			title = "None"
		}
		_, outdented := uniformOutdent(body, "", false, "", false)
		body = a.c.runBlockGamut("\n" + outdented + "\n")
		adm := uniformIndent(typ+"\n"+title+"\n\n"+body+"\n", a.c.tab, false, false)
		adm = `<aside class="` + class + "\">\n" + adm + "</aside>"
		return uniformIndent(adm, leadIndent, false, false)
	})
}

// ---- alerts

var alertRe = rx(`
        <blockquote>\s*
        <p>
        \[!(?P<type>NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]
        (?P<closing_tag></p>[ \t]*\n?)?
        (?P<contents>[\s\S]+?)
        </blockquote>
    `, reX)

type alerts struct{}

func (alerts) test(text string) bool { return strings.Contains(text, "<blockquote>") }

func (alerts) run(text string) string {
	return alertRe.Sub(text, func(m *pyre.Match) string {
		typ, _ := m.Named("type")
		contents, _ := m.Named("contents")
		contents = pyunicode.TrimSpace(contents)
		heading := "<em>" + typ[:1] + pyunicode.Lower(typ[1:]) + "</em>"
		low := pyunicode.Lower(typ)
		if closing, ok := m.Named("closing_tag"); ok && closing != "" {
			return `<div class="alert ` + low + "\">\n" + heading + "\n" + contents + "\n</div>"
		}
		return `<div class="alert ` + low + "\">\n" + heading + "\n<p>" + contents + "\n</div>"
	})
}

// ---- breaks

type breaks struct {
	c       *Converter
	options any
}

func (b *breaks) test(string) bool { return true }

func (b *breaks) run(text string) string {
	onBackslash := truthy(optGet(b.options, "on_backslash", false))
	onNewline := truthy(optGet(b.options, "on_newline", false))
	var pattern string
	switch {
	case onBackslash && onNewline:
		pattern = ` *\\?`
	case onBackslash:
		pattern = `(?: *\\| {2,})`
	case onNewline:
		pattern = ` *`
	default:
		pattern = ` {2,}`
	}
	return rx(pattern+`\n(?!\<(?:\/?(ul|ol|li))\>)`, 0).ReplaceLiteral(text, "<br"+b.c.emptyElementSuffix+"\n")
}

// ---- code-friendly

type codeFriendly struct {
	c         *Converter
	gfm       *gfmEmphasis
	hashTable *orderedMap
}

func newCodeFriendly(c *Converter, options any) extra {
	cf := &codeFriendly{c: c, hashTable: newOrderedMap()}
	cf.gfm = &gfmEmphasis{c: c, spanHook: cf.processSpan}
	return cf
}

func (cf *codeFriendly) hash(text string) string {
	for _, k := range cf.hashTable.keys {
		if cf.hashTable.m[k] == text {
			return k
		}
	}
	h := hashText("code-friendly" + text)
	cf.hashTable.set(h, text)
	return h
}

var underscoresRe = rx(`_+`, 0)

func (cf *codeFriendly) processSpan(open, close, middle *pyre.Match, openSyntax, closeSyntax string) ([]string, int, bool, bool) {
	text := slice(open.S, open.Start(0), close.End(0))
	openSyntax = orSyntax(openSyntax, open.Group(1))
	closeSyntax = orSyntax(closeSyntax, close.Group(1))
	if strings.Contains(openSyntax, "_") {
		text = underscoresRe.Sub(text, func(m *pyre.Match) string { return cf.hash(m.Text()) })
		return []string{text}, 0, false, true
	}
	if strings.Contains(text, "_") {
		inner := slice(text, len(openSyntax), len(text)-len(closeSyntax))
		inner = strings.ReplaceAll(inner, "__", cf.hash("__"))
		inner = strings.ReplaceAll(inner, "_", cf.hash("_"))
		return []string{openSyntax, inner, closeSyntax}, 0, false, true
	}
	return nil, 0, false, false
}

var hashSearchRe = rx(`md5-[0-9a-z]{32}`, 0)

func (cf *codeFriendly) test(text string) bool {
	return cf.gfm.test(text) || cf.hashTable.len() > 0 && hashSearchRe.Search(text, 0) != nil
}

func (cf *codeFriendly) run(text string) string {
	if cf.c.order < 2*int(StageItalicAndBold) {
		return cf.gfm.run(text)
	}
	orig := ""
	for orig != text {
		orig = text
		for _, k := range cf.hashTable.keys {
			text = strings.ReplaceAll(text, k, cf.hashTable.m[k])
		}
	}
	return text
}

// ---- emojis

type emojis struct{}

func newEmojis(c *Converter, options any) extra {
	if options != nil {
		if _, ok := options.(map[string]any); !ok {
			raise("AttributeError", "'%s' object has no attribute 'setdefault'", typeName(options))
		}
	}
	return emojis{}
}

var emojiRe = rx(`:[\S]+:`, 0)

func (emojis) test(text string) bool { return emojiRe.Search(text, 0) != nil }

func (emojis) run(string) string {
	raise("ImportError", `the "emoji" extra requires the "emoji" package to be installed`)
	return ""
}

// ---- fenced code blocks (and mermaid)

var fencedCodeBlockRe = rx(`
        (?:\n+|\A\n?|(?<=\n))
        (^[ \t]*`+"`"+`{3,})\s{0,99}?([\w+-]+)?\s{0,99}?\n  # $1 = opening fence (captured for back-referencing), $2 = optional lang
        (.*?)                             # $3 = code block content
        \1[ \t]*\n                      # closing fence
        `, reM|reX|reS)

var trailingNewlinesRe = rx(`\n*$`, 0)

type fencedCodeBlocks struct {
	c       *Converter
	mermaid bool
}

func (f *fencedCodeBlocks) test(text string) bool {
	if !strings.Contains(text, "```") {
		return false
	}
	if f.c.stage == StagePreprocess && f.c.safeMode == "" {
		return true
	}
	if f.c.stage == StageLinkDefs && f.c.safeMode != "" {
		return true
	}
	return f.c.stage == StageBlockGamut
}

func (f *fencedCodeBlocks) tags(lexer string) (string, string) {
	if f.mermaid && lexer == "mermaid" {
		return `<pre class="mermaid-pre"><div class="mermaid">`, "</div></pre>"
	}
	pre := f.c.htmlClassStrFromTag("pre")
	var code string
	if f.c.has("highlightjs-lang") && lexer != "" {
		code = ` class="` + lexer + ` language-` + lexer + `"`
	} else {
		code = f.c.htmlClassStrFromTag("code")
	}
	return "<pre" + pre + "><code" + code + ">", "</code></pre>"
}

func (f *fencedCodeBlocks) sub(m *pyre.Match) string {
	lexer := m.Group(2)
	codeblock := m.Group(3)
	if codeblock != "" {
		codeblock = codeblock[:len(codeblock)-1]
	}
	leadingNewlines := m.S[m.Start(0):m.Start(1)]
	trailing := trailingNewlinesRe.Search(m.Text(), 0).Text()
	fence := m.Group(1)
	leadingIndent := strings.Repeat(" ", len([]rune(fence))-len([]rune(pyunicode.TrimLeftSpace(fence))))
	if codeblock != "" {
		leadingIndent, codeblock = uniformOutdent(codeblock, "", false, leadingIndent, true)
	}
	codeblock = f.c.encodeCode(codeblock)
	open, close := f.tags(lexer)
	return leadingNewlines + leadingIndent + open + codeblock + "\n" + leadingIndent + close + trailing
}

func (f *fencedCodeBlocks) run(text string) string { return fencedCodeBlockRe.Sub(text, f.sub) }

// ---- latex

type latex struct{}

func (latex) test(string) bool { return true }

func (latex) run(string) string {
	raise("ImportError", `The "latex" extra requires the "latex2mathml" package to be installed.`)
	return ""
}

// ---- link patterns

type linkPatterns struct {
	c   *Converter
	raw any
}

var basicLinkRe = rx(`!?\[.*?\]\(.*?\)`, 0)

func (l *linkPatterns) test(string) bool { return true }

// pySliceEq is Python's text[a:b] == want for slice bounds that may be negative.
func pySliceEq(text string, a, b int, want string) bool {
	if a < 0 {
		a += len(text)
		if a < 0 {
			a = 0
		}
	}
	return slice(text, a, b) == want
}

func (l *linkPatterns) run(text string) string {
	c := l.c
	patterns := c.linkPatterns
	if _, given := l.raw.(givenLinkPatterns); !given {
		patterns = nil
		switch raw := l.raw.(type) {
		case nil:
			return text
		case []any:
			if len(raw) == 0 {
				return text
			}
			if pair, ok := raw[0].([]any); ok && len(pair) == 2 {
				raise("AttributeError", "'%s' object has no attribute 'finditer'", typeName(pair[0]))
			}
			raise("ValueError", "not enough values to unpack (expected 2)")
		case string:
			if raw == "" {
				return text
			}
			raise("ValueError", "not enough values to unpack (expected 2, got 1)")
		case map[string]any:
			if len(raw) == 0 {
				return text
			}
			raise("ValueError", "not enough values to unpack (expected 2)")
		default:
			raise("TypeError", "'%s' object is not iterable", typeName(raw))
		}
	}
	linkFromHash := newOrderedMap()
	for _, lp := range patterns {
		type repl struct {
			start, end int
			href       string
		}
		var replacements []repl
		for _, m := range lp.re.FindAll(text) {
			overlap := false
			for _, h := range linkFromHash.keys {
				if matchOverlapsSubstr(text, m, h) {
					overlap = true
					break
				}
			}
			if overlap {
				continue
			}
			var href string
			if lp.fn != nil {
				href = lp.fn(m)
			} else {
				expanded, err := m.Expand(lp.href)
				if err != nil {
					raise("re.error", "%s", err.Error())
				}
				href = expanded
			}
			replacements = append(replacements, repl{m.Start(0), m.End(0), href})
		}
		for i := len(replacements) - 1; i >= 0; i-- {
			start, end, href := replacements[i].start, replacements[i].end, replacements[i].href
			if pySliceEq(text, start-1, start, "[") && slice(text, end, end+1) == "]" {
				continue
			}
			if pySliceEq(text, start-2, start, "](") || slice(text, end, end+2) == `")` {
				continue
			}
			if pySliceEq(text, start-3, start, `"""`) && slice(text, end, end+3) == `"""` {
				text = text[:max(start-3, 0)] + text[start:end] + text[min(end+3, len(text)):]
				continue
			}
			inside := false
			for _, re := range []*pyre.Regexp{autoLinkRe, basicLinkRe} {
				for _, m := range re.FindAll(text) {
					for g := 0; g <= m.NumGroups(); g++ {
						if m.Start(g) <= start && end <= m.End(g) {
							inside = true
						}
					}
					if inside {
						break
					}
				}
				if inside {
					break
				}
			}
			if inside {
				continue
			}
			star, _ := c.escapeTable.get("*")
			under, _ := c.escapeTable.get("_")
			escaped := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(href, `"`, "&quot;"), "*", star), "_", under)
			link := `<a href="` + escaped + `">` + text[start:end] + "</a>"
			hash := c.hashSpan(link, linkFromHash)
			text = text[:start] + hash + text[end:]
		}
	}
	for _, h := range append([]string(nil), linkFromHash.keys...) {
		text = strings.ReplaceAll(text, h, linkFromHash.m[h])
	}
	return text
}

// ---- markdown in html

type markdownInHTML struct{ c *Converter }

func (m *markdownInHTML) test(string) bool { return true }

func (m *markdownInHTML) run(text string) string {
	return m.c.strictTagBlockSub(text, blockTagsA, func(block string) string {
		indent, b := uniformOutdent(block, "", false, "", false)
		b = m.c.hashHTMLBlockSub(b, "", false)
		return uniformIndent(b, indent, true, false)
	}, true)
}

// ---- markdown file links

type markdownFileLinks struct{ *linkProcessor }

func newMarkdownFileLinks(c *Converter, options any) extra {
	opts := map[string]any{"tags": []any{"a"}, "ref": false}
	if truthy(options) {
		m, ok := options.(map[string]any)
		if !ok {
			raise("TypeError", "'%s' object is not a mapping", typeName(options))
		}
		for k, v := range m {
			opts[k] = v
		}
	}
	lp := newLinkProcessor(c, opts)
	lp.mdFileLinks = true
	return &markdownFileLinks{lp}
}

func (m *markdownFileLinks) test(text string) bool {
	return m.linkProcessor.test(text) && strings.Contains(text, ".md")
}

// ---- middle-word-em

type middleWordEm struct {
	c       *Converter
	gfm     *gfmEmphasis
	allowed bool
	re      *pyre.Regexp
	hashes  map[string]string
}

func newMiddleWordEm(c *Converter, options any) extra {
	var opts map[string]any
	switch o := options.(type) {
	case bool:
		opts = map[string]any{"allowed": o}
	case map[string]any:
		opts = o
	default:
		if truthy(options) {
			raise("AttributeError", "'%s' object has no attribute 'setdefault'", typeName(options))
		}
		opts = map[string]any{}
	}
	allowed, ok := opts["allowed"]
	if !ok {
		allowed = true
		opts["allowed"] = true
	}
	var hashes []string
	for _, k := range c.escapeTable.keys {
		hashes = append(hashes, c.escapeTable.m[k])
	}
	escaped := strings.Join(hashes, "|")
	re := mustCompile(`
            (?<!^)         # To be middle of a word, it cannot be at the start of the input
            (?<![*_\W])    # cannot be preceeded by em char or non word char (must be in middle of word)
            (?<!`+escaped+`)        # cannot be preceeded by a hashed escape char either
            ([*_])         # em char
            (?=\S)         # must be followed by non-whitespace char
            (?!
                [*_]|$|\W  # cannot be followed by another em char, EOF or a non-word char
                |`+escaped+`        # Also cannot be followed by any of the escaped non-word chars
            )
            `, reX|reM)
	return &middleWordEm{c: c, gfm: &gfmEmphasis{c: c}, allowed: truthy(allowed), re: re,
		hashes: map[string]string{"_": hashText("middle-word-em_"), "*": hashText("middle-word-em*")}}
}

func (mw *middleWordEm) test(text string) bool {
	return mw.gfm.test(text) || hashSearchRe.Search(text, 0) != nil
}

func (mw *middleWordEm) run(text string) string {
	if mw.allowed {
		return text
	}
	if mw.c.order < 2*int(mw.c.stage) {
		text = mw.re.Sub(text, func(m *pyre.Match) string { return mw.hashes[m.Group(1)] })
	}
	if mw.c.order > 2*int(mw.c.stage) {
		text = strings.ReplaceAll(text, mw.hashes["_"], "_")
		text = strings.ReplaceAll(text, mw.hashes["*"], "*")
	}
	return text
}

// ---- numbering

var (
	numberingDefnsRe = rx(`
            \[\#(\w+) # the counter.  Open square plus hash plus a word \1
            ([^@]*)   # Some optional characters, that aren't an @. \2
            @(\w+)       # the id.  Should this be normed? \3
            ([^\]]*)\]   # The rest of the text up to the terminating ] \4
            `, reX)
	numberingSubsRe = rx(`\[@(\w+)\s*\]`, 0)
)

type numbering struct{ c *Converter }

func (n *numbering) test(string) bool { return true }

func (n *numbering) run(text string) string {
	counters := map[string]int{}
	type ref struct {
		number  int
		counter string
	}
	references := map[string]ref{}
	type repl struct {
		start int
		html  string
		end   int
	}
	var replacements []repl
	for _, m := range numberingDefnsRe.FindAll(text) {
		counter := m.Group(1)
		before := pyunicode.TrimSpace(m.Group(2))
		refID := m.Group(3)
		after := m.Group(4)
		number, ok := counters[counter]
		if !ok {
			number = 1
		}
		references[refID] = ref{number, counter}
		replacements = append(replacements, repl{m.Start(0),
			`<figcaption class="` + counter + `" id="counter-ref-` + refID + `">` + before + strconv.Itoa(number) + after + "</figcaption>",
			m.End(0)})
		counters[counter] = number + 1
	}
	for i := len(replacements) - 1; i >= 0; i-- {
		r := replacements[i]
		text = text[:r.start] + r.html + text[r.end:]
	}
	refs := numberingSubsRe.FindAll(text)
	for i := len(refs) - 1; i >= 0; i-- {
		m := refs[i]
		var html string
		if r, ok := references[m.Group(1)]; ok {
			html = `<a class="` + r.counter + `" href="#counter-ref-` + m.Group(1) + `">` + strconv.Itoa(r.number) + "</a>"
		} else {
			html = `<a class="` + m.Group(1) + `" href="#counter-ref-countererror">?` + m.Group(1) + "?</a>"
		}
		if n.c.has("smarty-pants") {
			q, _ := n.c.escapeTable.get(`"`)
			html = strings.ReplaceAll(html, `"`, q)
		}
		text = text[:m.Start(0)] + html + text[m.End(0):]
	}
	return text
}

// ---- pyshell

type pyshell struct{ c *Converter }

func (p *pyshell) test(text string) bool { return strings.Contains(text, ">>>") }

func (p *pyshell) run(text string) string {
	lessThanTab := p.c.tabWidth - 1
	re := mustCompile(`
            ^([ ]{0,`+strconv.Itoa(lessThanTab)+`})>>>[ ].*\n  # first line
            ^(\1[^\S\n]*\S.*\n)*    # any number of subsequent lines with at least one character
            (?=^\1?\n|\Z)           # ends with a blank line or end of document
            `, reM|reX)
	return re.Sub(text, func(m *pyre.Match) string {
		if p.c.has("fenced-code-blocks") {
			dedented := dedent(m.Text(), 8, false)
			fc, ok := p.c.extraClasses["fenced-code-blocks"]
			if !ok {
				raise("KeyError", "'fenced-code-blocks'")
			}
			return fc.run("```pycon\n" + dedented + "```\n")
		}
		lines := dedentLines(pyunicode.SplitLines(m.Text(), false), 8, false)
		indent := strings.Repeat(" ", max(p.c.tabWidth, 0))
		return "\n" + indent + strings.Join(lines, "\n"+indent) + "\n"
	})
}

// ---- smarty-pants

var (
	openingSingleQuoteRe = rx(`(?<!\S)'(?=\S)`, 0)
	openingDoubleQuoteRe = rx(`(?<!\S)"(?=\S)`, 0)
	closingSingleQuoteRe = rx(`(?<=\S)'`, 0)
	closingDoubleQuoteRe = rx(`(?<=\S)"(?=(\s|,|;|\.|\?|!|$))`, 0)
	apostropheYearRe     = rx(`'(\d\d)(?=(\s|,|;|\.|\?|!|$))`, 0)
	contractions         = []string{"tis", "twas", "twer", "neath", "o", "n", "round", "bout", "twixt", "nuff", "fraid", "sup"}
)

type smartyPants struct{ c *Converter }

func (s *smartyPants) test(text string) bool {
	for _, t := range []string{"'", `"`, "--", "...", ". . ."} {
		if strings.Contains(text, t) {
			return true
		}
	}
	return false
}

func (s *smartyPants) run(text string) string {
	tags := newOrderedMap()
	text = sortaHTMLTokenizeRe.Sub(text, func(m *pyre.Match) string {
		key := hashText(m.Text())
		tags.set(key, m.Text())
		return key
	})
	if strings.Contains(text, "'") {
		text = apostropheYearRe.MustSubTemplate(text, `&#8217;\1`)
		for _, c := range contractions {
			text = strings.ReplaceAll(text, "'"+c, "&#8217;"+c)
			capital := strings.ToUpper(c[:1]) + c[1:]
			text = strings.ReplaceAll(text, "'"+capital, "&#8217;"+capital)
		}
		text = openingSingleQuoteRe.ReplaceLiteral(text, "&#8216;")
		text = closingSingleQuoteRe.ReplaceLiteral(text, "&#8217;")
	}
	if strings.Contains(text, `"`) {
		text = openingDoubleQuoteRe.ReplaceLiteral(text, "&#8220;")
		text = closingDoubleQuoteRe.ReplaceLiteral(text, "&#8221;")
	}
	text = strings.ReplaceAll(text, "---", "&#8212;")
	text = strings.ReplaceAll(text, "--", "&#8211;")
	text = strings.ReplaceAll(text, "...", "&#8230;")
	text = strings.ReplaceAll(text, " . . . ", "&#8230;")
	text = strings.ReplaceAll(text, ". . .", "&#8230;")
	for _, k := range tags.keys {
		text = strings.ReplaceAll(text, k, tags.m[k])
	}
	return text
}

// ---- strike, tg-spoiler, underline

var (
	strikeRe    = rx(`~~(?=\S)(.+?)(?<=\S)~~`, reS)
	tgSpoilerRe = rx(`\|\|\s?(.+?)\s?\|\|`, reS)
	underlineRe = rx(`(?<!<!)--(?!>)(?=\S)(.+?)(?<=\S)(?<!<!)--(?!>)`, reS)
)

// templateExtra replaces matches of a pattern with a template, when the text holds a marker.
type templateExtra struct {
	re       *pyre.Regexp
	template string
	marker   string
}

func (t *templateExtra) test(text string) bool { return strings.Contains(text, t.marker) }

func (t *templateExtra) run(text string) string { return t.re.MustSubTemplate(text, t.template) }

// ---- tables

type tables struct{ c *Converter }

func (t *tables) test(string) bool { return true }

func (t *tables) run(text string) string {
	ltt := strconv.Itoa(t.c.tabWidth - 1)
	re := mustCompile(`
                (?:(?<=\n)|\A\n?)             # leading blank line

                ^[ ]{0,`+ltt+`}                      # allowed whitespace
                (.*[|].*)[ ]*\n                   # $1: header row (at least one pipe)

                ^[ ]{0,`+ltt+`}                      # allowed whitespace
                (                               # $2: underline row
                    # underline row with leading bar
                    (?:  \|\ *:?-+:?\ *  )+  \|? \s?[ ]*\n
                    |
                    # or, underline row without leading bar
                    (?:  \ *:?-+:?\ *\|  )+  (?:  \ *:?-+:?\ *  )? \s?[ ]*\n
                )

                (                               # $3: data rows
                    (?:
                        ^[ ]{0,`+ltt+`}(?!\ )         # ensure line begins with 0 to less_than_tab spaces
                        .*\|.*[ ]*\n
                    )*
                )
            `, reM|reX)
	return re.Sub(text, t.sub)
}

var (
	unescapedPipeRe = rx(`(?<!\\)\|`, 0)
	escapedPipeRe   = rx(`\\\|`, 0)
)

func splitRow(row string) []string {
	row = strings.TrimPrefix(pyunicode.TrimSpace(row), "|")
	if !strings.HasSuffix(row, `\|`) {
		row = strings.TrimSuffix(row, "|")
	}
	spans := codeSpanRe.FindAll(row)
	starts := map[int]bool{}
	ends := map[int]bool{}
	for _, m := range spans {
		starts[m.Start(2)] = true
		ends[m.End(0)] = true
	}
	var cells []string
	start := 0
	for _, m := range unescapedPipeRe.FindAll(row) {
		pos := m.Start(0)
		if pos > 0 && row[pos-1] == '`' {
			if starts[pos] || (!ends[pos] && strings.Count(row[:pos], "`")%2 == 1) {
				continue
			}
		}
		cells = append(cells, row[start:pos])
		start = m.End(0)
	}
	cells = append(cells, row[start:])
	for i, cell := range cells {
		cells[i] = escapedPipeRe.ReplaceLiteral(pyunicode.TrimSpace(cell), "|")
	}
	return cells
}

func (t *tables) sub(m *pyre.Match) string {
	head, underline, body := m.Group(1), m.Group(2), m.Group(3)
	delims := splitRow(underline)
	align := map[int]string{}
	for i, col := range delims {
		if col == "" {
			raise("IndexError", "string index out of range")
		}
		switch {
		case col[0] == ':' && col[len(col)-1] == ':':
			align[i] = ` style="text-align:center;"`
		case col[0] == ':':
			align[i] = ` style="text-align:left;"`
		case col[len(col)-1] == ':':
			align[i] = ` style="text-align:right;"`
		}
	}
	c := t.c
	hlines := []string{"<table" + c.htmlClassStrFromTag("table") + ">", "<thead" + c.htmlClassStrFromTag("thead") + ">", "<tr>"}
	headerCols := splitRow(head)
	if len(headerCols) != len(delims) {
		return m.Text()
	}
	columns := len(headerCols)
	for i, col := range headerCols {
		hlines = append(hlines, "  <th"+align[i]+">"+c.runSpanGamut(col)+"</th>")
	}
	hlines = append(hlines, "</tr>", "</thead>")
	body = strings.Trim(body, "\n")
	if body != "" {
		hlines = append(hlines, "<tbody>")
		for _, line := range strings.Split(body, "\n") {
			hlines = append(hlines, "<tr>")
			cols := splitRow(line)
			for len(cols) < columns {
				cols = append(cols, "")
			}
			cols = cols[:columns]
			for i, col := range cols {
				hlines = append(hlines, "  <td"+align[i]+">"+c.runSpanGamut(col)+"</td>")
			}
			hlines = append(hlines, "</tr>")
		}
		hlines = append(hlines, "</tbody>")
	}
	hlines = append(hlines, "</table>")
	return strings.Join(hlines, "\n") + "\n"
}

// ---- wavedrom

type wavedrom struct {
	c       *Converter
	options any
}

func (w *wavedrom) test(text string) bool {
	m := fencedCodeBlockRe.Search(text, 0)
	return m == nil || m.Group(2) == "wavedrom"
}

func (w *wavedrom) run(text string) string {
	return fencedCodeBlockRe.Sub(text, func(m *pyre.Match) string {
		leadIndent, waves := uniformOutdent(m.Group(3), "", false, "", false)
		optGet(w.options, "prefer_embed_svg", true)
		h := hashText(waves)
		w.c.escapeTable.set(waves, h)
		return uniformIndent("\n<script type=\"WaveDrom\">\n"+h+"</script>\n", leadIndent, true, false)
	})
}

// ---- wiki links

var wikiLinkRe = rx(`\[\[([\w0-9_ -]+)\]\]`, reU)

type wikiLinks struct {
	c       *Converter
	options any
}

func (w *wikiLinks) test(text string) bool { return strings.Contains(text, "[[") }

func (w *wikiLinks) run(text string) string {
	return wikiLinkRe.Sub(text, func(m *pyre.Match) string {
		label := pyunicode.TrimSpace(m.Group(1))
		if label == "" {
			return m.Text()
		}
		base := optGet(w.options, "base_url", "/")
		end := optGet(w.options, "end_url", "/")
		var url string
		switch build := optGet(w.options, "build_url", nil).(type) {
		case nil:
			url = pyStr(base) + strings.ReplaceAll(label, " ", "_") + pyStr(end)
		case WikiLinkURLBuilder:
			url = build(label, base, end)
		case func(string, any, any) string:
			url = build(label, base, end)
		default:
			raise("TypeError", "'%s' object is not callable", typeName(build))
		}
		url = strings.ReplaceAll(url, `"`, "&quot;")
		class := optGet(w.options, "html_class", "wikilink")
		classStr := ""
		if truthy(class) {
			classStr = ` class="` + pyStr(class) + `"`
		}
		return w.c.hashSpan(`<a href="`+url+`"`+classStr+">"+label+"</a>", nil)
	})
}

// ---- wiki tables

type wikiTables struct{ c *Converter }

func (w *wikiTables) test(text string) bool { return strings.Contains(text, "||") }

var (
	wikiCellSplitRe = rx(`(?<!\\)\|\|`, 0)
	wikiHeaderRe    = rx(`^\s*~`, 0)
)

func (w *wikiTables) run(text string) string {
	ltt := strconv.Itoa(w.c.tabWidth - 1)
	re := mustCompile(`
            (?:(?<=\n\n)|\A\n?)            # leading blank line
            ^([ ]{0,`+ltt+`})\|\|.+?\|\|[ ]*\n  # first line
            (^\1\|\|.+?\|\|\n)*        # any number of subsequent lines
            `, reM|reX)
	return re.Sub(text, w.sub)
}

func (w *wikiTables) sub(m *pyre.Match) string {
	c := w.c
	ttext := pyunicode.TrimSpace(m.Text())
	var rows [][]string
	for _, line := range pyunicode.SplitLines(ttext, false) {
		line = pyunicode.TrimSpace(line)
		rs := []rune(line)
		if len(rs) >= 4 {
			line = string(rs[2 : len(rs)-2])
		} else {
			line = ""
		}
		line = pyunicode.TrimSpace(line)
		var row []string
		for _, cell := range wikiCellSplitRe.Split(line, 0) {
			row = append(row, pyunicode.TrimSpace(cell))
		}
		rows = append(rows, row)
	}
	var hlines []string
	add := func(line string, indents int) {
		hlines = append(hlines, strings.Repeat(c.tab, indents)+line)
	}
	format := func(cell string) string {
		return c.runSpanGamut(strings.Trim(wikiHeaderRe.ReplaceLiteral(cell, ""), " "))
	}
	add("<table"+c.htmlClassStrFromTag("table")+">", 0)
	if len(rows) > 0 && len(rows[0]) > 0 && wikiHeaderRe.MatchString(rows[0][0]) != nil {
		add("<thead"+c.htmlClassStrFromTag("thead")+">", 1)
		add("<tr>", 2)
		for _, cell := range rows[0] {
			add("<th>"+format(cell)+"</th>", 3)
		}
		add("</tr>", 2)
		add("</thead>", 1)
		rows = rows[1:]
	}
	if len(rows) > 0 {
		add("<tbody>", 1)
		for _, row := range rows {
			add("<tr>", 2)
			for _, cell := range row {
				add("<td>"+format(cell)+"</td>", 3)
			}
			add("</tr>", 2)
		}
		add("</tbody>", 1)
	}
	add("</table>", 0)
	return strings.Join(hlines, "\n") + "\n"
}

// ---- memoised block patterns

func hrTagRe(tabWidth int) *pyre.Regexp {
	return mustCompile(`
        (?:
            (?<=\n\n)       # Starting after a blank line
            |               # or
            \A\n?           # the beginning of the doc
        )
        (                       # save in \1
            [ ]{0,`+strconv.Itoa(tabWidth-1)+`}
            <(hr)               # start tag = \2
            \b                  # word break
            ([^<>])*?           #
            /?>                 # the matching end tag
            [ \t]*
            (?=\n{2,}|\Z)       # followed by a blank line or end of document
        )
        `, reX)
}

func xmlOnelinerRe(tabWidth int) *pyre.Regexp {
	return mustCompile(`
        (?:
            (?<=\n\n)       # Starting after a blank line
            |               # or
            \A\n?           # the beginning of the doc
        )
        (                           # save in $1
            [ ]{0,`+strconv.Itoa(tabWidth-1)+`}
            (?:
                <\?\w+\b\s+.*?\?>   # XML processing instruction
                |
                <\w+:\w+\b\s+.*?/>  # namespaced single tag
            )
            [ \t]*
            (?=\n{2,}|\Z)       # followed by a blank line or end of document
        )
        `, reX)
}
