// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

// Package markdown2 is a fast and complete implementation of Markdown, a text-to-HTML filter
// that translates an easy-to-read, easy-to-write structured text format into HTML. It implements
// the full core syntax plus a number of extras (code syntax fencing, footnotes, tables, header
// ids, a table of contents, smart quotes, ...) described at
// https://github.com/trentm/python-markdown2/wiki/Extras.
//
//	html, err := markdown2.Markdown("*boo!*", markdown2.Options{})
//	// html.HTML == "<p><em>boo!</em></p>\n"
package markdown2

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/trentm/python-markdown2/internal/pyhtml"
	"github.com/trentm/python-markdown2/internal/pyre"
	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// Version is the markdown2 version this package implements.
const Version = "2.5.6"

// DefaultTabWidth is the tab width used when Options.TabWidth is zero.
const DefaultTabWidth = 4

// LinkPattern auto-links text matching Pattern (the "link-patterns" extra). Pattern is either
// "/regex/flags" (a Python-syntax regular expression with flags from "ilsmu") or a literal
// string. The link's href is Func's result when Func is set, otherwise Href, a replacement
// template that may refer to groups (\1, \g<name>).
type LinkPattern struct {
	Pattern string
	Href    string
	Func    func(m LinkMatch) string
}

// LinkMatch is a link pattern's match.
type LinkMatch interface {
	// Text returns the whole matched text.
	Text() string
	// Group returns the text of a numbered group ("" when it did not take part in the match).
	Group(i int) string
	// Named returns the text of a named group and whether it took part in the match.
	Named(name string) (string, bool)
}

// WikiLinkURLBuilder is the type of the "build_url" option of the "wiki-links" extra: it returns
// the href for a link label, given the base_url and end_url options.
type WikiLinkURLBuilder func(label string, baseURL, endURL any) string

// Options configure a conversion; they mirror the keyword arguments of markdown2.markdown().
type Options struct {
	// HTML4Tags writes empty elements as <br> instead of <br />.
	HTML4Tags bool
	// TabWidth is the tab stop width (DefaultTabWidth when zero).
	TabWidth int
	// SafeMode sanitises raw HTML: "replace" or "escape" ("" leaves HTML alone).
	SafeMode string
	// Extras maps an extra's name to its argument: nil for none, or a bool, number, string,
	// []any or map[string]any (the values encoding/json produces).
	Extras map[string]any
	// LinkPatterns are the patterns of the "link-patterns" extra.
	LinkPatterns []LinkPattern
	// FootnoteTitle is the title of footnote back links, a printf-style template given the
	// footnote's id and number.
	FootnoteTitle string
	// FootnoteReturnSymbol is the text of footnote back links.
	FootnoteReturnSymbol string
	// UseFileVars enables extras named by an Emacs-style "markdown-extras" file variable.
	UseFileVars bool
	// CLI marks a conversion run by the command line tool (the table of contents is prepended).
	CLI bool
}

// Result is a conversion's output.
type Result struct {
	HTML string
	// TOCHTML is the table of contents when the "toc" extra found headers.
	TOCHTML *string
	// Metadata holds the document's metadata when the "metadata" extra is on.
	Metadata map[string]any
}

// String returns the HTML.
func (r *Result) String() string { return r.HTML }

// Stage is a step of the conversion; extras run before or after stages.
type Stage int

// The conversion stages, in order.
const (
	StagePreprocess Stage = iota + 1
	StageHashHTML
	StageLinkDefs
	StageBlockGamut
	StageHeaders
	StageLists
	StageCodeBlocks
	StageBlockQuotes
	StageParagraphs
	StageSpanGamut
	StageCodeSpans
	StageEscapeSpecial
	StageLinks
	StageItalicAndBold
	StagePostprocess
	StageUnhashHTML
)

type tocEntry struct {
	level int
	id    string
	name  string
}

// givenLinkPatterns stands, as the "link-patterns" extra's argument, for Options.LinkPatterns.
type givenLinkPatterns struct{}

type linkPattern struct {
	re   *pyre.Regexp
	href string
	fn   func(m LinkMatch) string
}

// Converter converts Markdown documents with one set of options; state such as header id
// counters (with the "header-ids" reset-count option off) carries over between conversions.
type Converter struct {
	emptyElementSuffix   string
	tabWidth             int
	tab                  string
	safeMode             string
	extras               map[string]any
	instanceExtras       map[string]any
	extraClasses         map[string]extra
	linkPatterns         []linkPattern
	footnoteTitle        string
	footnoteReturnSymbol string
	useFileVars          bool
	cli                  bool

	urls       map[string]string
	titles     map[string]string
	htmlBlocks map[string]string
	htmlSpans  *orderedMap
	listLevel  int
	stage      Stage
	order      int // twice the stage, minus one before it and plus one after it

	toc                []tocEntry
	tocDepth           any
	escapeTable        *orderedMap
	codeTable          *orderedMap
	footnotes          *orderedMap
	footnoteIDs        []string
	footnoteNumbers    map[string]int
	footnoteMarker     string
	countFromHeaderID  map[string]int
	headerIDsSeen      map[string]bool
	metadata           map[string]any
	lastLiEndsWithEOLs bool
	flankCache         map[flankKey][2]bool
}

var gEscapeTable = func() *orderedMap {
	t := newOrderedMap()
	for _, ch := range "\\`*_{}[]()>#+-.!" {
		t.set(string(ch), hashText(string(ch)))
	}
	return t
}()

// Markdown converts text with the given options.
func Markdown(text string, opts Options) (*Result, error) {
	c, err := New(opts)
	if err != nil {
		return nil, err
	}
	return c.Convert(text)
}

// NewWithExtras returns a Converter with the "footnotes" and "fenced-code-blocks" extras on, in
// addition to the extras of the options.
func NewWithExtras(opts Options) (*Converter, error) {
	extras := map[string]any{"footnotes": nil, "fenced-code-blocks": nil}
	for k, v := range opts.Extras {
		extras[k] = v
	}
	opts.Extras = extras
	return New(opts)
}

// MarkdownPath converts the file at path, decoded with the named encoding ("" for UTF-8).
func MarkdownPath(path, encoding string, opts Options) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, err := Decode(data, encoding)
	if err != nil {
		return nil, err
	}
	return Markdown(text, opts)
}

// catch turns a conversion failure raised inside the package into a returned error.
func catch(err *error) {
	if r := recover(); r != nil {
		switch e := r.(type) {
		case *Error:
			*err = e
		case *MarkdownError:
			*err = e
		default:
			panic(r)
		}
	}
}

// New returns a Converter for the options.
func New(opts Options) (c *Converter, err error) {
	defer catch(&err)
	c = &Converter{emptyElementSuffix: " />", tabWidth: opts.TabWidth, safeMode: opts.SafeMode, flankCache: map[flankKey][2]bool{}}
	if opts.HTML4Tags {
		c.emptyElementSuffix = ">"
	}
	if c.tabWidth == 0 {
		c.tabWidth = DefaultTabWidth
	}
	if c.tabWidth < 0 {
		c.tab = ""
	} else {
		c.tab = strings.Repeat(" ", c.tabWidth)
	}
	c.extras = map[string]any{}
	for k, v := range opts.Extras {
		c.extras[k] = v
	}
	c.setupTOC()
	c.normalizeHeaderIDs()
	if _, ok := c.extras["break-on-newline"]; ok {
		breaks, isDict := c.extras["breaks"].(map[string]any)
		if !isDict {
			breaks = map[string]any{}
		} else {
			breaks = copyDict(breaks)
		}
		breaks["on_newline"] = true
		c.extras["breaks"] = breaks
	}
	for _, p := range opts.LinkPatterns {
		re, err := regexFromEncodedPattern(p.Pattern)
		if err != nil {
			return nil, err
		}
		c.linkPatterns = append(c.linkPatterns, linkPattern{re, p.Href, p.Func})
	}
	if lp, ok := c.extras["link-patterns"]; ok {
		switch {
		case len(c.linkPatterns) > 0:
			c.extras["link-patterns"] = givenLinkPatterns{}
		case lp == nil:
			return nil, &MarkdownError{Message: "If the 'link-patterns' extra is used, an argument for 'link_patterns' is required"}
		}
	}
	c.instanceExtras = copyDict(c.extras)
	c.footnoteTitle = opts.FootnoteTitle
	c.footnoteReturnSymbol = opts.FootnoteReturnSymbol
	c.useFileVars = opts.UseFileVars
	c.cli = opts.CLI
	c.escapeTable = gEscapeTable.clone()
	c.codeTable = newOrderedMap()
	if _, ok := c.extras["smarty-pants"]; ok {
		c.escapeTable.set(`"`, hashText(`"`))
		c.escapeTable.set("'", hashText("'"))
	}
	return c, nil
}

func copyDict(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *Converter) has(name string) bool {
	_, ok := c.extras[name]
	return ok
}

func (c *Converter) reset() {
	c.urls = map[string]string{}
	c.titles = map[string]string{}
	c.htmlBlocks = map[string]string{}
	c.htmlSpans = newOrderedMap()
	c.listLevel = 0
	c.extras = copyDict(c.instanceExtras)
	c.setupExtras()
	c.toc = nil
}

func (c *Converter) setupTOC() {
	if !c.has("toc") {
		return
	}
	if !c.has("header-ids") {
		c.extras["header-ids"] = nil
	}
	if c.extras["toc"] == nil {
		c.tocDepth = 6
	} else {
		c.tocDepth = optGet(c.extras["toc"], "depth", 6)
	}
}

func (c *Converter) normalizeHeaderIDs() {
	if !c.has("header-ids") {
		return
	}
	if _, ok := c.extras["header-ids"].(map[string]any); !ok {
		c.extras["header-ids"] = map[string]any{"mixed": false, "prefix": c.extras["header-ids"], "reset-count": true}
	}
}

func (c *Converter) headerIDsOpts() map[string]any {
	m, _ := c.extras["header-ids"].(map[string]any)
	return m
}

func (c *Converter) setupExtras() {
	c.setupTOC()
	c.normalizeHeaderIDs()
	if c.has("footnotes") {
		c.footnotes = newOrderedMap()
		c.footnoteIDs = nil
		c.footnoteNumbers = map[string]int{}
		c.footnoteMarker = hashText("<<footnote>>")
	}
	if c.has("header-ids") {
		reset := truthy(c.headerIDsOpts()["reset-count"])
		if c.countFromHeaderID == nil || reset {
			c.countFromHeaderID = map[string]int{}
		}
		if c.headerIDsSeen == nil || reset {
			c.headerIDsSeen = map[string]bool{}
		}
	}
	if c.has("metadata") {
		c.metadata = map[string]any{}
	}
	c.extraClasses = map[string]extra{}
	for _, reg := range extraRegistry {
		if !c.has(reg.name) {
			continue
		}
		c.extraClasses[reg.name] = reg.make(c, c.extras[reg.name])
	}
}

// runStage runs fn as the given stage, with the extras registered before and after it.
func (c *Converter) runStage(st Stage, text string, fn func(string) string) string {
	c.stage = st
	c.order = 2*int(st) - 1
	order := execOrder[st]
	for _, name := range order.before {
		if e, ok := c.extraClasses[name]; ok && e.test(text) {
			text = e.run(text)
		}
	}
	c.order = 2 * int(st)
	text = fn(text)
	c.order = 2*int(st) + 1
	for _, name := range order.after {
		if e, ok := c.extraClasses[name]; ok && e.test(text) {
			text = e.run(text)
		}
	}
	return text
}

var aNofollowOrBlankLinks = rx(`
        <(a)
        (
            [^>]*
            href=   # href is required
            ['"]?   # HTML5 attribute values do not have to be quoted
            [^#'"]  # We don't want to match href values that start with # (like footnotes)
        )
        `, reI|reX)

var wsOnlyLineRe = rx(`^[ \t]+$`, reM)

// Convert converts one document.
func (c *Converter) Convert(text string) (res *Result, err error) {
	defer catch(&err)
	c.reset()

	if c.useFileVars {
		text = emacsOnelinerVarsPat.Sub(text, c.emacsVarsOnelinerSub)
		emacsVars := c.getEmacsVars(text)
		if v, ok := emacsVars["markdown-extras"]; ok {
			for _, e := range rx(`[ ,]+`, 0).Split(v, 0) {
				if name, arg, found := strings.Cut(e, "="); found {
					if n, ok := pyunicode.ParseInt(arg); ok {
						c.extras[name] = n
					} else {
						c.extras[name] = arg
					}
				} else {
					c.extras[e] = nil
				}
			}
		}
		c.setupExtras()
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text += "\n\n"
	text = c.detab(text)
	text = wsOnlyLineRe.ReplaceLiteral(text, "")

	if c.has("metadata") {
		text = c.extractMetadata(text)
	}

	text = c.runStage(StagePreprocess, text, func(t string) string { return t })

	if c.safeMode != "" {
		text = c.hashHTMLSpans(text)
	}

	text = c.hashHTMLBlocks(text, true)

	if c.has("footnotes") {
		text = c.stripFootnoteDefinitions(text)
	}
	text = c.stripLinkDefinitions(text)

	text = c.runBlockGamut(text)

	if c.has("footnotes") {
		text = c.doFootnoteMarker(text)
		text = c.addFootnotes(text)
	}

	text = c.runStage(StagePostprocess, text, func(t string) string { return t })

	text = c.unescapeSpecialChars(text)
	text = c.unhashHTMLSpans(text, true, false)
	if c.safeMode != "" {
		text = strings.ReplaceAll(text, htmlRemovedText, htmlRemovedTextCompat)
	}

	blank, nofollow := c.has("target-blank-links"), c.has("nofollow")
	switch {
	case blank && nofollow:
		text = aNofollowOrBlankLinks.MustSubTemplate(text, `<\1 rel="nofollow noopener" target="_blank"\2`)
	case blank:
		text = aNofollowOrBlankLinks.MustSubTemplate(text, `<\1 rel="noopener" target="_blank"\2`)
	case nofollow:
		text = aNofollowOrBlankLinks.MustSubTemplate(text, `<\1 rel="nofollow"\2`)
	}

	var tocHTML string
	if c.has("toc") && len(c.toc) > 0 {
		if truthy(c.headerIDsOpts()["mixed"]) {
			c.sortTOC(text)
		}
		tocHTML = calculateTOCHTML(c.toc)
		prepend := false
		if c.extras["toc"] != nil {
			prepend = truthy(optGet(c.extras["toc"], "prepend", false))
		}
		if c.cli || prepend {
			text = tocHTML + "\n" + text
		}
	}
	text += "\n"

	res = &Result{HTML: text}
	if c.has("toc") && len(c.toc) > 0 {
		res.TOCHTML = &tocHTML
	}
	if c.has("metadata") {
		res.Metadata = c.metadata
	}
	return res, nil
}

// sortTOC orders the entries by where their header appears in the HTML (mixed headers).
func (c *Converter) sortTOC(text string) {
	keys := make([]int, len(c.toc))
	for i, e := range c.toc {
		pattern := `^<(h` + strconv.Itoa(e.level) + `).*?id=(["\'])` + e.id + `\2.*>` + pyre.Escape(e.name) + `</\1>$`
		re, err := pyre.Compile(pattern, reM)
		if err != nil {
			raise("re.error", "%s", err.Error())
		}
		if m := re.Search(text, 0); m != nil {
			keys[i] = utf8.RuneCountInString(text[:m.Start(0)])
		}
	}
	idx := make([]int, len(c.toc))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	sorted := make([]tocEntry, len(c.toc))
	for i, j := range idx {
		sorted[i] = c.toc[j]
	}
	c.toc = sorted
}

func (c *Converter) doFootnoteMarker(text string) string {
	re := rx(c.footnoteMarker+`-(.*?)(?=</a></sup>)`, 0)
	return re.Sub(text, func(m *pyre.Match) string {
		normed := m.Group(1)
		if _, ok := c.footnoteNumbers[normed]; !ok {
			c.footnoteIDs = append(c.footnoteIDs, normed)
			c.footnoteNumbers[normed] = len(c.footnoteNumbers) + 1
		}
		return strconv.Itoa(c.footnoteNumbers[normed])
	})
}

// ---- metadata

var metaDataPattern = rx(`
        ^`+metaFence+`(  # optional opening fence
            (?:
                `+metaKey+`:(?:\n+[ \t]+.*)+  # indented lists
            )|(?:
                `+metaKey+`:\s+>(?:\n+[ \t]+.*)+  # multiline long descriptions
            )|(?:
                `+metaKey+`:[^\n]*\n?  # simple key:value pair, leading spaces allowed
            )
        )`+metaFence+`  # optional closing fence
        `, reM|reX)

const (
	metaFence = `(?:---[\ \t]*\n)?`
	metaKey   = `[ \t]*[^\s:\r\n][^:\r\n]*`
)

var (
	keyValListPat        = rx(`^-[ \t]*([^\n]*)(?:\n((?:[ \t]+[^\n]+\n?)+))?`, reM)
	keyValDictPat        = rx(`^([^:\n]+)[ \t]*:[ \t]*([^\n]*)(?:((?:\n[ \t]+[^\n]+)+))?`, reM)
	metaDataFencePattern = rx(`^---[\ \t]*\n`, reM)
	metaDataNewline      = rx("^\n", reM)
)

func (c *Converter) extractMetadata(text string) string {
	var content, tail string
	if strings.HasPrefix(text, "---") {
		splits := metaDataFencePattern.Split(text, 2)
		if len(splits) < 3 {
			return text
		}
		content, tail = splits[1], splits[2]
	} else {
		splits := metaDataNewline.Split(text, 1)
		content = splits[0]
		if len(splits) > 1 {
			tail = splits[1]
		}
	}
	matches := metaDataPattern.FindAll(content)
	if len(matches) == 0 {
		return text
	}
	for _, m := range matches {
		item := m.Group(1)
		k, v, _ := strings.Cut(item, ":")
		key := pyunicode.TrimSpace(k)
		switch {
		case strings.HasPrefix(v, " >\n"):
			c.metadata[key] = pyunicode.TrimSpace(dedent(v[3:], 8, false))
		case v == "\n":
			c.metadata[key] = ""
		case strings.HasPrefix(v, "\n"):
			c.metadata[key] = parseStructuredValue(v, v)
		default:
			if v == "" {
				raise("IndexError", "string index out of range")
			}
			c.metadata[key] = pyunicode.TrimSpace(v)
		}
	}
	return tail
}

// parseStructuredValue parses an indented metadata block. v is the value of the top-level item
// being parsed (the Python original measures the indent to strip against it at every level).
func parseStructuredValue(value, v string) any {
	vs := pyunicode.TrimLeftSpace(value)
	cut := utf8.RuneCountInString(value) - utf8.RuneCountInString(vs)
	vs = dropFirstRune(strings.ReplaceAll(value, headRunes(v, cut), "\n"))

	if strings.HasPrefix(vs, "-") {
		r := []any{}
		for _, m := range keyValListPat.FindAll(vs) {
			val, block := m.Group(1), m.Group(2)
			switch {
			case val != "" && block == "":
				r = append(r, pyunicode.TrimSpace(val))
			case val == ">" && block != "":
				r = append(r, pyunicode.TrimSpace(block))
			case val == "" && block != "":
				r = append(r, parseStructuredValue(block, v))
			}
		}
		return r
	}
	d := map[string]any{}
	for _, m := range keyValDictPat.FindAll(vs) {
		if m.Group(2) != "" {
			d[pyunicode.TrimSpace(m.Group(1))] = pyunicode.TrimSpace(m.Group(2))
		} else {
			d[pyunicode.TrimSpace(m.Group(1))] = parseStructuredValue(m.Group(3), v)
		}
	}
	return d
}

// ---- Emacs file variables

var emacsOnelinerVarsPat = rx(`((?:<!--)?\s*-\*-)\s*(?:(\S[^\r\n]*?)([\r\n]\s*)?)?(-\*-\s*(?:-->)?)`, reU)

var emacsLocalVarsPat = rx(`^
        (?P<prefix>(?:[^\r\n|\n|\r])*?)
        [\ \t]*Local\ Variables:[\ \t]*
        (?P<suffix>.*?)(?:\r\n|\n|\r)
        (?P<content>.*?\1End:)
        `, reI|reM|reS|reX)

func (c *Converter) emacsVarsOnelinerSub(m *pyre.Match) string {
	if pyunicode.TrimSpace(m.Group(1)) == "-*-" && pyunicode.TrimSpace(m.Group(4)) == "-*-" {
		leadWS := rx(`^\s*`, 0).MatchString(m.Group(1)).Text()
		tailWS := rx(`\s*$`, 0).FindAll(m.Group(4))[0].Text()
		return leadWS + "<!-- -*- " + pyunicode.TrimSpace(m.Group(2)) + " -*- -->" + tailWS
	}
	return m.Text()
}

// debugf records a debug message of the "markdown" logger (shown by the command line tool's -v).
var debugf = func(format string, args ...any) {}

// SetDebugLogger sets the function that receives the library's debug messages.
func SetDebugLogger(f func(msg string)) {
	debugf = func(format string, args ...any) { f(pyFormat(format, args...)) }
}

func (c *Converter) getEmacsVars(text string) map[string]string {
	vars := map[string]string{}
	var order []string
	setVar := func(k, v string) {
		if _, ok := vars[k]; !ok {
			order = append(order, k)
		}
		vars[k] = v
	}
	const size = 8192
	head := headRunes(text, size)
	if strings.Contains(head, "-*-") {
		if m := emacsOnelinerVarsPat.Search(head, 0); m != nil {
			varsStr, ok := m.GroupOK(2)
			if !ok {
				raise("TypeError", "argument of type 'NoneType' is not iterable")
			}
			if strings.Contains(varsStr, "\n") {
				raise("AssertionError", "")
			}
			var strs []string
			for _, s := range strings.Split(varsStr, ";") {
				if pyunicode.TrimSpace(s) != "" {
					strs = append(strs, pyunicode.TrimSpace(s))
				}
			}
			if len(strs) == 1 && !strings.Contains(strs[0], ":") {
				setVar("mode", pyunicode.TrimSpace(strs[0]))
			} else {
				for _, s := range strs {
					variable, value, found := strings.Cut(pyunicode.TrimSpace(s), ":")
					if !found {
						debugf("emacs variables error: malformed -*- line: %r", s)
						continue
					}
					setVar(pyunicode.Lower(variable), pyunicode.TrimSpace(value))
				}
			}
		}
	}

	tail := tailRunes(text, size)
	if strings.Contains(tail, "Local Variables") {
		if m := emacsLocalVarsPat.Search(tail, 0); m != nil {
			prefix, _ := m.Named("prefix")
			suffix, _ := m.Named("suffix")
			content, _ := m.Named("content")
			lines := pyunicode.SplitLines(content, false)
			for i, line := range lines {
				if !strings.HasPrefix(line, prefix) {
					debugf("emacs variables error: line '%s' does not use proper prefix '%s'", line, prefix)
					return map[string]string{}
				}
				if i != len(lines)-1 && !strings.HasSuffix(line, suffix) {
					debugf("emacs variables error: line '%s' does not use proper suffix '%s'", line, suffix)
					return map[string]string{}
				}
			}
			continuedFor := ""
			hasContinued := false
			for _, line := range lines[:max(len(lines)-1, 0)] {
				if prefix != "" {
					line = line[len(prefix):]
				}
				if suffix != "" {
					line = line[:len(line)-len(suffix)]
				}
				line = pyunicode.TrimSpace(line)
				if hasContinued {
					variable := continuedFor
					if strings.HasSuffix(line, "\\") {
						line = pyunicode.TrimRightSpace(line[:len(line)-1])
					} else {
						hasContinued = false
					}
					setVar(variable, vars[variable]+" "+line)
				} else {
					variable, value, found := strings.Cut(line, ":")
					if !found {
						debugf("local variables error: missing colon in local variables entry: '%s'", line)
						continue
					}
					value = pyunicode.TrimSpace(value)
					if strings.HasSuffix(value, "\\") {
						value = pyunicode.TrimRightSpace(value[:len(value)-1])
						continuedFor, hasContinued = variable, true
					} else {
						hasContinued = false
					}
					setVar(variable, value)
				}
			}
		}
	}
	for _, k := range order {
		val := vars[k]
		if utf8.RuneCountInString(val) > 1 && strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
			vars[k] = val[1 : len(val)-1]
		}
	}
	return vars
}

// ---- tabs

func (c *Converter) detabLine(line string) string {
	for {
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			return line
		}
		chunk1, chunk2 := line[:i], line[i+1:]
		if c.tabWidth == 0 {
			raise("ZeroDivisionError", "integer modulo by zero")
		}
		n := c.tabWidth - pyMod(utf8.RuneCountInString(chunk1), c.tabWidth)
		if n > 0 {
			chunk1 += strings.Repeat(" ", n)
		}
		line = chunk1 + chunk2
	}
}

func pyMod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}

func (c *Converter) detab(text string) string {
	if !strings.Contains(text, "\t") {
		return text
	}
	lines := pyunicode.SplitLines(text, false)
	for i, line := range lines {
		lines[i] = c.detabLine(line)
	}
	return strings.Join(lines, "\n")
}

// ---- HTML blocks

const (
	voidTags              = `area|base|br|col|embed|hr|img|input|link|meta|param|source|track|wbr`
	html5Tags             = `|address|article|aside|canvas|figcaption|figure|footer|header|main|nav|section|video`
	blockTagsA            = `blockquote|body|dd|del|div|dl|dt|fieldset|form|h[1-6]|head|hr|html|iframe|ins|li|math|noscript|ol|p|pre|script|style|table|tfoot|ul` + html5Tags
	blockTagsB            = `blockquote|div|dl|fieldset|form|h[1-6]|head|iframe|math|noscript|ol|p|pre|script|style|table|ul` + html5Tags
	spanTags              = `a|abbr|acronym|b|bdo|big|br|button|cite|code|dfn|em|i|img|input|kbd|label|map|object|output|q|samp|script|select|small|span|strong|sub|sup|textarea|time|tt|var`
	htmlRemovedText       = "{(#HTML#)}"
	htmlRemovedTextCompat = "[HTML_REMOVED]"
)

var liberalTagBlockRe = rx(`
        (                       # save in \1
            ^                   # start of line  (with re.M)
            <(`+blockTagsB+`)               # start tag = \2
            \b                  # word break
            (.*\n)*?            # any number of lines, minimally matching
            .*</\2>             # the matching end tag
            [ \t]*              # trailing spaces/tabs
            (?=\n+|\Z)          # followed by a newline or end of document
        )
        `, reX|reM)

var htmlMarkdownAttrRe = rx(`\s+markdown(?:="1"|='1'|=1)?(?![^\s/>\b])`, 0)

var hTagRe = rx(`
        ^<h([1-6])(.*)>  # \1 tag num, \2 attrs
        (.*)  # \3 text
        </h\1>
    `, reX|reM)

// hashHTMLBlockSub hashes one HTML block; tag is "" when the caller has no tag group.
func (c *Converter) hashHTMLBlockSub(html, tag string, raw bool) string {
	if tag == "" {
		m := rx(`.*?<(\S).*?\s*>`, reS).MatchString(html)
		if m == nil {
			raise("AssertionError", "")
		}
		tag = m.Group(1)
	}
	if raw && c.safeMode != "" {
		html = c.sanitizeHTML(html)
	} else if c.has("markdown-in-html") && strings.Contains(html, "markdown") {
		firstLine, _, _ := strings.Cut(html, "\n")
		if m := htmlMarkdownAttrRe.Search(firstLine, 0); m != nil {
			lines := strings.Split(html, "\n")
			var head []string
			for _, s := range mustCompile(`(.*?<`+tag+`.*markdown.*?>)`, 0).Split(lines[0], 0) {
				if s != "" {
					head = append(head, s)
				}
			}
			lines = append(head, lines[1:]...)
			var tailParts []string
			for _, s := range mustCompile(`(\s*?</`+tag+`>.*?$)`, 0).Split(lines[len(lines)-1], 0) {
				if s != "" {
					tailParts = append(tailParts, s)
				}
			}
			lines = append(lines[:len(lines)-1:len(lines)-1], tailParts...)
			firstLine = lines[0]
			middle := strings.Join(lines[1:max(len(lines)-1, 1)], "\n")
			lastLine := lines[len(lines)-1]
			firstLine = firstLine[:m.Start(0)] + firstLine[m.End(0):]
			fKey := hashText(firstLine)
			c.htmlBlocks[fKey] = firstLine
			lKey := hashText(lastLine)
			c.htmlBlocks[lKey] = lastLine
			return "\n\n" + fKey + "\n\n" + middle + "\n\n" + lKey + "\n\n"
		}
	} else if hid := c.headerIDsOpts(); hid != nil && truthy(hid["mixed"]) && hTagRe.MatchString(html) != nil {
		html = hTagRe.Sub(html, c.hTagSub)
	}
	key := hashText(html)
	c.htmlBlocks[key] = html
	return "\n\n" + key + "\n\n"
}

// mustCompile compiles a pattern built from document text; an invalid one fails the conversion
// as re.error, as in Python.
func mustCompile(pattern string, flags pyre.Flags) *pyre.Regexp {
	re, err := pyre.Compile(pattern, flags)
	if err != nil {
		raise("re.error", "%s", err.Error())
	}
	return re
}

func (c *Converter) hashHTMLBlocks(text string, raw bool) string {
	return c.runStage(StageHashHTML, text, func(text string) string {
		if !strings.Contains(text, "<") {
			return text
		}
		sub := func(m *pyre.Match) string {
			tag := ""
			if m.NumGroups() >= 2 {
				tag = m.Group(2)
			}
			return c.hashHTMLBlockSub(m.Group(1), tag, raw)
		}
		text = c.strictTagBlockSub(text, blockTagsA, func(b string) string { return c.hashHTMLBlockSub(b, "", raw) }, false)
		text = liberalTagBlockRe.Sub(text, sub)
		text = c.strictTagBlockSub(text, spanTags, func(b string) string {
			return c.hashHTMLBlockSub(c.runSpanGamut(b), "", raw)
		}, false)
		if strings.Contains(text, "<hr") {
			text = hrTagRe(c.tabWidth).Sub(text, sub)
		}
		if strings.Contains(text, "<!--") {
			text = c.hashHTMLComments(text, raw)
		}
		if c.has("xml") {
			text = xmlOnelinerRe(c.tabWidth).Sub(text, sub)
		}
		return text
	})
}

func (c *Converter) hashHTMLComments(text string, raw bool) string {
	start := 0
	for {
		si := strings.Index(text[min(start, len(text)):], "<!--")
		if si < 0 {
			return text
		}
		startIdx := si + min(start, len(text))
		ei := strings.Index(text[startIdx:], "-->")
		if ei < 0 {
			return text
		}
		endIdx := startIdx + ei + 3
		start = endIdx
		if startIdx != 0 {
			for i := 0; i < c.tabWidth-1; i++ {
				if text[startIdx-1] != ' ' {
					break
				}
				startIdx--
				if startIdx == 0 {
					break
				}
			}
			switch {
			case startIdx == 0:
			case startIdx == 1 && text[0] == '\n':
				startIdx = 0
			case startIdx >= 2 && text[startIdx-2:startIdx] == "\n\n":
			default:
				return text
			}
		}
		for endIdx < len(text) && (text[endIdx] == ' ' || text[endIdx] == '\t') {
			endIdx++
		}
		next := text[endIdx:min(endIdx+2, len(text))]
		if next != "" && next != "\n" && next != "\n\n" {
			continue
		}
		html := text[startIdx:endIdx]
		if raw && c.safeMode != "" {
			html = c.sanitizeHTML(html)
		}
		key := hashText(html)
		c.htmlBlocks[key] = html
		text = text[:startIdx] + "\n\n" + key + "\n\n" + text[endIdx:]
	}
}

// strictTagBlockSub finds HTML blocks opened and closed by the same tag at the start of a line
// (balanced over nested tags) and replaces each with callback's result.
func (c *Converter) strictTagBlockSub(text, tagsRe string, callback func(string) string, allowIndent bool) string {
	tagCount := 0
	currentTag := tagsRe
	var block, result strings.Builder
	indent := "0"
	if allowIndent {
		indent = ""
	}
	for _, chunk := range pyunicode.SplitLines(text, true) {
		isMarkup := mustCompile(`^(\s{0,`+indent+`})(?:</code>(?=</pre>))?(</?(`+currentTag+`)\b>?)`, 0).MatchString(chunk)
		block.WriteString(chunk)
		if isMarkup != nil {
			if c.tagIsClosed(isMarkup.Group(3), chunk) || (tagCount == 0 && strings.HasPrefix(isMarkup.Group(2), "</")) {
				isMarkup = nil
			} else {
				currentTag = isMarkup.Group(3)
				tagCount += c.tagImbalance(currentTag, chunk)
			}
		} else if currentTag != tagsRe && strings.Contains(chunk, currentTag) {
			tagCount += c.tagImbalance(currentTag, chunk)
		}
		if tagCount == 0 {
			b := block.String()
			if isMarkup != nil {
				b = callback(strings.TrimRight(b, "\n"))
			}
			currentTag = tagsRe
			result.WriteString(b)
			block.Reset()
		}
	}
	result.WriteString(block.String())
	return result.String()
}

func (c *Converter) tagIsClosed(tagName, text string) bool {
	if mustCompile(voidTags, 0).MatchString(tagName) != nil {
		return true
	}
	openTag := "<" + tagName
	openCount := 0
	for _, line := range pyunicode.SplitLines(text, false) {
		pos := strings.Index(line, openTag)
		for pos != -1 {
			end := strings.Index(line[pos:], ">")
			if end == -1 {
				break
			}
			end += pos
			openCount++
			p := strings.Index(line[end+1:], openTag)
			if p < 0 {
				pos = -1
			} else {
				pos = end + 1 + p
			}
		}
	}
	if openCount != strings.Count(text, "</"+tagName+">") {
		return false
	}
	closeIndex := strings.Index(text, "</"+tagName)
	openIndex := strings.Index(text, "<"+tagName)
	return openIndex != -1 && closeIndex != -1 && openIndex < closeIndex
}

func (c *Converter) tagImbalance(tagName, text string) int {
	if mustCompile(voidTags, 0).MatchString(tagName) != nil {
		return 0
	}
	count := 0
	for _, m := range mustCompile(`<(/)?`+tagName+`\b>?`, 0).FindAll(text) {
		if _, ok := m.GroupOK(1); ok && m.Group(1) != "" {
			count--
		} else {
			count++
		}
	}
	return count
}

// ---- link and footnote definitions

func (c *Converter) stripLinkDefinitions(text string) string {
	return c.runStage(StageLinkDefs, text, func(text string) string {
		lessThanTab := c.tabWidth - 1
		re := mustCompile(`
            ^[ ]{0,`+strconv.Itoa(lessThanTab)+`}\[(.+)\]: # id = \1
              [ \t]*
              \n?               # maybe *one* newline
              [ \t]*
            <?(.+?)>?           # url = \2
              [ \t]*
            (?:
                \n?             # maybe one newline
                [ \t]*
                (?<=\s)         # lookbehind for whitespace
                ['"(]
                ([^\n]*)        # title = \3
                ['")]
                [ \t]*
            )?  # title is optional
            (?:\n+|\Z)
            `, reX|reM|reU)
		return re.Sub(text, func(m *pyre.Match) string {
			key := pyunicode.Lower(m.Group(1))
			c.urls[key] = c.encodeAmpsAndAngles(m.Group(2))
			if title := m.Group(3); title != "" {
				c.titles[key] = title
			}
			return ""
		})
	})
}

func (c *Converter) stripFootnoteDefinitions(text string) string {
	lessThanTab := c.tabWidth - 1
	re := mustCompile(`
            ^[ ]{0,`+strconv.Itoa(lessThanTab)+`}\[\^(.+)\]:   # id = \1
            [ \t]*
            (                       # footnote text = \2
              # First line need not start with the spaces.
              (?:\s*.*\n+)
              (?:
                (?:[ ]{`+strconv.Itoa(c.tabWidth)+`} | \t)  # Subsequent lines must be indented.
                .*\n+
              )*
            )
            # Lookahead for non-space at line-start, or end of doc.
            (?:(?=^[ ]{0,`+strconv.Itoa(c.tabWidth)+`}\S)|\Z)
            `, reX|reM)
	return re.Sub(text, func(m *pyre.Match) string {
		id, body := m.Group(1), m.Group(2)
		body = pyunicode.TrimSpace(dedent(body, 8, !strings.HasPrefix(body, "\n")))
		normed := rx(`\W`, 0).ReplaceLiteral(id, "-")
		c.footnotes.set(normed, body+"\n\n")
		return ""
	})
}

// ---- block gamut

var hrRe = rx(`^[ ]{0,3}([-_*])[ ]{0,2}(\1[ ]{0,2}){2,}$`, reM)

func (c *Converter) runBlockGamut(text string) string {
	return c.runStage(StageBlockGamut, text, func(text string) string {
		text = c.doHeaders(text)
		text = hrRe.ReplaceLiteral(text, "\n<hr"+c.emptyElementSuffix+"\n")
		text = c.doLists(text)
		text = c.doCodeBlocks(text)
		text = c.doBlockQuotes(text)
		text = c.hashHTMLBlocks(text, false)
		text = c.formParagraphs(text)
		return text
	})
}

var hardBreakRe = rx(` {2,}\n(?!\<(?:\/?(ul|ol|li))\>)`, 0)

func (c *Converter) runSpanGamut(text string) string {
	return c.runStage(StageSpanGamut, text, func(text string) string {
		text = c.doCodeSpans(text)
		text = c.escapeSpecialChars(text)
		text = c.doLinks(text)
		text = c.doAutoLinks(text)
		text = c.encodeAmpsAndAngles(text)
		text = c.doItalicsAndBold(text)
		text = hardBreakRe.ReplaceLiteral(text, "<br"+c.emptyElementSuffix+"\n")
		return text
	})
}

var sortaHTMLTokenizeRe = rx(`
        (
            \\*  # escapes
            (?:
                # tag
                </?
                (?:\w+)         # tag name
                (?:             # attributes
                    \s+                           # whitespace after tag
                    [^\s<>"'=/][^<>"'=/]*=        # attr name, can't start with whitespace
                    (?:"[^"]*?"|'[^']*?'|[^<>"'=/\s]+)  # value, quoted or unquoted. If unquoted, no spaces allowed
                )*
                \s*/?>
                |
                # auto-link (e.g., <http://www.activestate.com/>)
                <[\w~:/?#\[\]@!$&'\(\)*+,;%=\.\\-]+>
                |
                <!--.*?-->      # comment
                |
                <\?.*?\?>       # processing instruction
            )
        )
        `, reX)

var isUnescapedRe = rx(`^((?:\\\\)*(?!\\))`, 0)

func (c *Converter) escapeSpecialChars(text string) string {
	return c.runStage(StageEscapeSpecial, text, func(text string) string {
		var escaped strings.Builder
		isHTMLMarkup := false
		for _, token := range sortaHTMLTokenizeRe.Split(text, 0) {
			if isHTMLMarkup && isUnescapedRe.MatchString(token) != nil {
				parts := isUnescapedRe.Split(token, 0)
				escapeSeq, tok := "", token
				if len(parts) > 1 {
					escapeSeq, tok = parts[1], parts[2]
				}
				star, _ := c.escapeTable.get("*")
				under, _ := c.escapeTable.get("_")
				bs, _ := c.escapeTable.get("\\")
				escaped.WriteString(strings.ReplaceAll(escapeSeq, `\\`, bs) +
					strings.ReplaceAll(strings.ReplaceAll(tok, "*", star), "_", under))
			} else {
				escaped.WriteString(c.encodeBackslashEscapes(
					strings.ReplaceAll(strings.ReplaceAll(token, `\<`, "&lt;"), `\>`, "&gt;")))
			}
			isHTMLMarkup = !isHTMLMarkup
		}
		return escaped.String()
	})
}

func (c *Converter) isAutoLink(text string) bool {
	if strings.Contains(text, ":") {
		if m := autoLinkRe.MatchString(text); m != nil {
			return c.safeMode == "" || c.safeHref().MatchString(m.Group(1)) != nil
		}
	}
	return strings.Contains(text, "@") && autoEmailLinkRe.MatchString(text) != nil
}

var (
	codeSpanHashRe = rx(`<code>md5-[A-Fa-f0-9]{32}</code>`, 0)
	commentRe      = rx(`(<!--)(.*)(-->)`, 0)
)

func (c *Converter) hashHTMLSpans(text string) string {
	return c.runStage(StageHashHTML, text, func(text string) string {
		codeHashes := newOrderedMap()
		ltHash := hashText("&lt;")
		c.codeTable.set("&lt;", ltHash)
		text = codeSpanRe.Sub(text, func(m *pyre.Match) string {
			return c.hashSpan(strings.ReplaceAll(m.Text(), "<", ltHash), codeHashes)
		})

		split := sortaHTMLTokenizeRe.Split(text, 0)
		isCode := func(index int, token string) bool {
			var peek []string
			switch token {
			case "<code>":
				peek = split[min(index, len(split)):min(index+3, len(split))]
			case "</code>":
				lo := index - 2
				if lo < 0 {
					// Python's negative slice start counts from the end.
					lo += len(split)
				}
				if lo > index+1 {
					peek = nil
				} else {
					peek = split[lo : index+1]
				}
			default:
				return false
			}
			return codeSpanHashRe.MatchString(strings.Join(peek, "")) != nil
		}
		var tokens []string
		for index := 0; index < len(split); {
			isMarkup := index%2 != 0
			token := split[index]
			code := isCode(index, token)
			switch {
			case isMarkup && !c.isAutoLink(token) && !code:
				var comment *pyre.Match
				if c.safeMode != "replace" {
					comment = commentRe.MatchString(token)
				}
				switch {
				case comment != nil:
					tokens = append(tokens, c.hashSpan(c.sanitizeHTML(comment.Group(1)), nil))
					tokens = append(tokens, c.sanitizeHTML(comment.Group(2)))
					tokens = append(tokens, c.hashSpan(c.sanitizeHTML(comment.Group(3)), nil))
				case isUnescapedRe.MatchString(token) == nil:
					tokens = append(tokens, c.sanitizeHTML(c.escapeSpecialChars(token)))
				default:
					tokens = append(tokens, c.hashSpan(c.sanitizeHTML(token), nil))
				}
			case isMarkup && code:
				tokens = append(tokens, split[index:min(index+3, len(split))]...)
				index += 3
				continue
			default:
				tokens = append(tokens, c.encodeIncompleteTags(token))
			}
			index++
		}
		text = strings.Join(tokens, "")
		for _, key := range codeHashes.keys {
			text = strings.ReplaceAll(text, key, codeHashes.m[key])
		}
		return text
	})
}

func (c *Converter) unhashHTMLSpans(text string, spans, code bool) string {
	orig := ""
	for text != orig {
		orig = text
		if spans {
			for _, key := range append([]string(nil), c.htmlSpans.keys...) {
				text = strings.ReplaceAll(text, key, c.htmlSpans.m[key])
			}
		}
		if code {
			for _, codeText := range append([]string(nil), c.codeTable.keys...) {
				text = strings.ReplaceAll(text, c.codeTable.m[codeText], codeText)
			}
		}
	}
	return text
}

var ampersandRe = rx(`&(?!`+ampersandBody+`)`, 0)

const ampersandBody = `#?[xX]?(?:[0-9a-fA-F]+|\w+);`

var escapedAmpersandRe = rx(`(?:\\\\)*\\&(`+ampersandBody+`)`, 0)

func (c *Converter) sanitizeHTML(s string) string {
	switch c.safeMode {
	case "replace":
		return htmlRemovedText
	case "escape":
		s = ampersandRe.ReplaceLiteral(s, "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		return strings.ReplaceAll(s, ">", "&gt;")
	}
	raise("MarkdownError", "invalid value for 'safe_mode': %s (must be 'escape' or 'replace')", pyunicode.Repr(c.safeMode))
	return ""
}

var (
	inlineLinkTitle = rx(`
            (                   # \1
              [ \t]+
              (['"])            # quote char = \2
              (?P<title>.*?)
              \2
            )?                  # title is optional
          \)$
        `, reX|reS)
	tailOfReferenceLinkRe = rx(`
          # Match tail of: [text][id]
          [ ]?          # one optional space
          (?:\n[ ]*)?   # one optional newline followed by spaces
          \[
            (?P<id>[^\[\]]*?)
          \]
        `, reX|reS)
	whitespaceRe       = rx(`\s*`, 0)
	stripAngleBrackets = rx(`<(.*)>.*`, 0)
	dataURLRe          = rx(`
        data:
        # in format type/subtype;parameter=optional
        (?P<mime>\w+/[\w+\.-]+(?:;\w+=[\w+\.-]+)?)?
        # optional base64 token
        (?P<token>;base64)?
        ,(?P<data>.*)
    `, reX)
)

func (c *Converter) findNonWhitespace(text string, start int) int {
	if m := whitespaceRe.MatchAt(text, start); m != nil {
		return m.End(0)
	}
	return len(text)
}

func findBalanced(text string, start int, open, close byte) int {
	i, count := start, 1
	for count > 0 && i < len(text) {
		switch text[i] {
		case open:
			count++
		case close:
			count--
		}
		i++
	}
	return i
}

// protectURL escapes a URL and hashes it so later stages leave it alone.
func (c *Converter) protectURL(url string) string {
	if url == "" {
		return url
	}
	// data: URLs are escaped as they are; any other URL first gets its hashed spans back and its
	// emphasis characters protected.
	if dataURLRe.MatchString(url) == nil {
		star, _ := c.escapeTable.get("*")
		under, _ := c.escapeTable.get("_")
		url = strings.ReplaceAll(strings.ReplaceAll(c.unhashHTMLSpans(url, true, true), "*", star), "_", under)
	}
	url = htmlEscapeURL(url, c.safeMode != "")
	key := hashText(url)
	c.escapeTable.set(url, key)
	return key
}

const safeProtocols = `(?:https?|ftp):\/\/|(?:mailto|tel):`

// safeHref matches the hrefs safe mode lets through.
func (c *Converter) safeHref() *pyre.Regexp {
	safe := `-\w`
	lessSafe := `#/\.!#$%&\(\)\+,/:;=\?@\[\]^` + "`" + `\{\}\|~`
	seps := strings.Join([]string{":", `&#x0*3a;?`, `&#0*58;?`, `&colon;?`}, "|")
	domain := `(?:[` + safe + `]+(?:\.[` + safe + `]+)*)(?:(?<!tel)(?<!javascript):\d+/?)?(?![^:/]*(?:` + seps + `)/*)`
	fragment := `[` + safe + lessSafe + `]*`
	return rx(`^(?:(`+safeProtocols+`)?(`+domain+`)(`+fragment+`)|(#|\.{,2}/)(`+fragment+`))$`, reI)
}

func (c *Converter) doLinks(text string) string {
	return c.runStage(StageLinks, text, func(text string) string {
		lp := newLinkProcessor(c, nil)
		if lp.test(text) {
			text = lp.run(text)
		}
		return text
	})
}

// headerIDFromText makes the id of a header from its HTML text.
func (c *Converter) headerIDFromText(text string, prefix any) string {
	headerID := slugify(pyhtml.Unescape(text))
	if p, ok := prefix.(string); ok && p != "" {
		headerID = p + "-" + headerID
	}
	baseID := headerID
	c.countFromHeaderID[baseID]++
	if baseID == "" || c.countFromHeaderID[baseID] > 1 {
		headerID = baseID + "-" + strconv.Itoa(c.countFromHeaderID[baseID])
	}
	for c.headerIDsSeen[headerID] {
		c.countFromHeaderID[baseID]++
		headerID = baseID + "-" + strconv.Itoa(c.countFromHeaderID[baseID])
	}
	c.headerIDsSeen[headerID] = true
	return headerID
}

func (c *Converter) headerIDExists(text string) bool {
	headerID := slugify(text)
	if p, ok := c.headerIDsOpts()["prefix"].(string); ok && p != "" {
		headerID = p + "-" + headerID
	}
	if _, ok := c.countFromHeaderID[headerID]; ok {
		return true
	}
	for _, e := range c.toc {
		if e.id == headerID {
			return true
		}
	}
	return false
}

func (c *Converter) tocAddEntry(level int, id, name string) {
	depth, ok := asInt(c.tocDepth)
	if !ok {
		raise("TypeError", "'>' not supported between instances of 'int' and '%s'", typeName(c.tocDepth))
	}
	if level > depth {
		return
	}
	c.toc = append(c.toc, tocEntry{level, id, c.unescapeSpecialChars(name)})
}

const hReBase = `
        (^(.+)[ \t]{0,99}\n(=+|-+)[ \t]*\n+)
        |
        (^(\#{1,6})  # \1 = string of #'s
        [ \t]%s
        (.+?)       # \2 = Header text
        [ \t]{0,99}
        (?<!\\)     # ensure not an escaped trailing '#'
        \#*         # optional closing #'s (not counted)
        \n+
        )
        `

var (
	hRe            = rx(strings.Replace(hReBase, "%s", "*", 1), reX|reM)
	hReTagFriendly = rx(strings.Replace(hReBase, "%s", "+", 1), reX|reM)
)

func (c *Converter) hSub(m *pyre.Match) string {
	var n int
	var headerGroup string
	if g1, ok := m.GroupOK(1); ok && m.Group(3) == "-" {
		return g1
	} else if ok {
		if m.Group(3)[0] == '=' {
			n = 1
		} else {
			n = 2
		}
		headerGroup = m.Group(2)
	} else {
		n = len(m.Group(5))
		headerGroup = m.Group(6)
	}
	if d := c.extras["demote-headers"]; truthy(d) {
		dn, ok := asInt(d)
		if !ok {
			raise("TypeError", "unsupported operand type(s) for +: 'int' and '%s'", typeName(d))
		}
		n = min(n+dn, 6)
	}
	headerIDAttr := ""
	headerID := ""
	if c.has("header-ids") {
		headerText := c.unhashHTMLSpans(headerGroup, true, false)
		headerID = c.headerIDFromText(c.unescapeSpecialChars(headerText), c.headerIDsOpts()["prefix"])
		if headerID != "" {
			headerIDAttr = ` id="` + headerID + `"`
		}
	}
	html := c.runSpanGamut(headerGroup)
	if c.has("toc") && headerID != "" {
		c.tocAddEntry(n, headerID, html)
	}
	return "<h" + strconv.Itoa(n) + headerIDAttr + ">" + html + "</h" + strconv.Itoa(n) + ">\n\n"
}

func (c *Converter) hTagSub(m *pyre.Match) string {
	text := m.Text()
	level, _ := strconv.Atoi(m.Group(1))
	idAttr := ""
	if im := rx(`.*?id="(\S+)?".*`, 0).MatchString(m.Group(2)); im != nil {
		idAttr = im.Group(1)
	}
	idAttr = strings.Trim(idAttr, `'" `)
	hText := m.Group(3)
	if idAttr != "" && c.headerIDExists(idAttr) {
		return text
	}
	headerID := idAttr
	if headerID == "" {
		headerID = c.headerIDFromText(hText, c.headerIDsOpts()["prefix"])
	}
	if c.has("toc") {
		c.tocAddEntry(level, headerID, hText)
	}
	if headerID != "" && idAttr == "" {
		return text[:3] + ` id="` + headerID + `"` + text[3:]
	}
	return text
}

func (c *Converter) doHeaders(text string) string {
	return c.runStage(StageHeaders, text, func(text string) string {
		if c.has("tag-friendly") {
			return hReTagFriendly.Sub(text, c.hSub)
		}
		return hRe.Sub(text, c.hSub)
	})
}

// ---- lists

const (
	markerULChars = "*+-"
	markerAny     = `(?:[*+-]|\d+\.)`
	markerUL      = `(?:[*+-])`
	markerOL      = `(?:\d+\.)`
)

func (c *Converter) listSub(m *pyre.Match) string {
	lst := m.Group(1)
	marker := m.Group(4)
	lstType := "ol"
	if strings.Contains(markerULChars, marker) {
		lstType = "ul"
	}
	opts := ""
	if lstType == "ol" && marker != "1." {
		opts = ` start="` + marker[:len(marker)-1] + `"`
	}
	opts += c.htmlClassStrFromTag(lstType)
	result := c.processListItems(lst)
	if c.listLevel != 0 {
		return "<" + lstType + opts + ">\n" + result + "</" + lstType + ">\n"
	}
	return "<" + lstType + opts + ">\n" + result + "</" + lstType + ">\n\n"
}

func (c *Converter) doLists(text string) string {
	return c.runStage(StageLists, text, func(text string) string {
		pos := 0
		for {
			var best *pyre.Match
			for _, markerPat := range []string{markerUL, markerOL} {
				lessThanTab := c.tabWidth - 1
				other := markerOL
				if markerPat == markerOL {
					other = markerUL
				}
				wholeList := `
                    (                   # \1 = whole list
                      (                 # \2
                        ([ ]{0,` + strconv.Itoa(lessThanTab) + `})     # \3 = the indentation level of the list item marker
                        (` + markerPat + `)            # \4 = first list item marker
                        [ \t]+
                        (?!\ *\4\ )     # '- - - ...' isn't a list. See 'not_quite_a_list' test case.
                      )
                      (?:.+?)
                      (                 # \5
                          \Z
                        |
                          \n{2,}
                          (?=\S)
                          (?!           # Negative lookahead for another list item marker
                            [ \t]*
                            ` + markerPat + `[ \t]+
                          )
                        |
                          \n+
                          (?=
                            \3          # lookahead for a different style of list item marker
                            ` + other + `[ \t]+
                          )
                      )
                    )
                `
				var re *pyre.Regexp
				if c.listLevel != 0 {
					re = mustCompile("^"+wholeList, reX|reM|reS)
				} else {
					re = mustCompile(`(?:(?<=\n\n)|\A\n?)`+wholeList, reX|reM|reS)
				}
				if m := re.Search(text, pos); m != nil && (best == nil || m.Start(0) < best.Start(0)) {
					best = m
				}
			}
			if best == nil {
				break
			}
			start, end := best.Start(0), best.End(0)
			middle := c.listSub(best)
			text = text[:start] + middle + text[end:]
			pos = start + len(middle)
		}
		return text
	})
}

var (
	listItemRe = rx(`
        (\n)?                   # leading line = \1
        (^[ \t]*)               # leading whitespace = \2
        (?P<marker>`+markerAny+`) [ \t]+   # list marker = \3
        ((?:.*?)                # list item text = \4 (may be empty)
        (\n{1,2}))              # eols = \5
        (?= \n* (\Z | \2 (?P<next_marker>`+markerAny+`) [ \t]+))
        `, reM|reX|reS)
	taskListItemRe = rx(`
        (\[[\ xX]\])[ \t]+       # tasklist marker = \1
        (.*)                   # list item text = \2
    `, reM|reX|reS)
)

func taskListItemSub(m *pyre.Match) string {
	marker, itemText := m.Group(1), m.Group(2)
	switch marker {
	case "[x]", "[X]":
		return `<input type="checkbox" class="task-list-item-checkbox" checked disabled> ` + itemText
	case "[ ]":
		return `<input type="checkbox" class="task-list-item-checkbox" disabled> ` + itemText
	}
	return ""
}

func (c *Converter) listItemSub(m *pyre.Match) string {
	item := m.Group(4)
	_, leading := m.GroupOK(1)
	if leading || strings.Contains(item, "\n\n") || c.lastLiEndsWithEOLs {
		_, item = uniformOutdent(item, " ", true, c.tab, true)
		item = c.runBlockGamut(item)
	} else {
		_, out := uniformOutdent(item, " ", true, "", false)
		item = c.doLists(out)
		item = strings.TrimSuffix(item, "\n")
		item = c.runSpanGamut(item)
	}
	c.lastLiEndsWithEOLs = len(m.Group(5)) == 2
	if c.has("task_list") {
		item = taskListItemRe.Sub(item, taskListItemSub)
	}
	return "<li>" + item + "</li>\n"
}

func (c *Converter) processListItems(listStr string) string {
	c.listLevel++
	c.lastLiEndsWithEOLs = false
	listStr = strings.TrimRight(listStr, "\n") + "\n"
	listStr = listItemRe.Sub(listStr, c.listItemSub)
	c.listLevel--
	return listStr
}

// ---- code blocks

func (c *Converter) outdent(text string) string {
	return mustCompile(`^(\t|[ ]{1,`+strconv.Itoa(c.tabWidth)+`})`, reM).ReplaceLiteral(text, "")
}

func (c *Converter) codeBlockSub(m *pyre.Match) string {
	codeblock := c.outdent(m.Group(1))
	codeblock = c.detab(codeblock)
	codeblock = strings.TrimLeft(codeblock, "\n")
	codeblock = pyunicode.TrimRightSpace(codeblock)
	pre := c.htmlClassStrFromTag("pre")
	code := c.htmlClassStrFromTag("code")
	codeblock = c.encodeCode(codeblock)
	return "\n<pre" + pre + "><code" + code + ">" + codeblock + "\n</code></pre>\n"
}

func (c *Converter) htmlClassStrFromTag(tag string) string {
	classes, ok := c.extras["html-classes"].(map[string]any)
	if !ok {
		return ""
	}
	if v, ok := classes[tag]; ok {
		return ` class="` + pyStr(v) + `"`
	}
	return ""
}

func (c *Converter) doCodeBlocks(text string) string {
	return c.runStage(StageCodeBlocks, text, func(text string) string {
		tw := strconv.Itoa(c.tabWidth)
		re := mustCompile(`
            (?:\n\n|\A\n?)
            (               # $1 = the code block -- one or more lines, starting with a space/tab
              (?:
                (?:[ ]{`+tw+`} | \t)  # Lines must start with a tab or a tab-width of spaces
                .*\n+
              )+
            )
            ((?=^[ ]{0,`+tw+`}\S)|\Z)   # Lookahead for non-space at line-start, or end of doc
            # Lookahead to make sure this block isn't already in a code block.
            # Needed when syntax highlighting is being used.
            (?!([^<]|<(/?)span)*\</code\>)
            `, reM|reX)
		return re.Sub(text, c.codeBlockSub)
	})
}

var codeSpanRe = rx(`
            (?<!\\)
            (`+"`"+`+)        # \1 = Opening run of `+"`"+`
            (?!`+"`"+`)       # See Note A test/tm-cases/escapes.text
            (.+?)       # \2 = The code block
            (?<!`+"`"+`)
            \1          # Matching closer
            (?!`+"`"+`)
        `, reX)

func (c *Converter) codeSpanSub(m *pyre.Match) string {
	code := strings.Trim(m.Group(2), " \t")
	return "<code" + c.htmlClassStrFromTag("code") + ">" + c.encodeCode(code) + "</code>"
}

func (c *Converter) doCodeSpans(text string) string {
	return c.runStage(StageCodeSpans, text, func(text string) string {
		return codeSpanRe.Sub(text, c.codeSpanSub)
	})
}

func (c *Converter) encodeCode(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	hashed := hashText(text)
	c.codeTable.set(text, hashed)
	return hashed
}

func (c *Converter) doItalicsAndBold(text string) string {
	return c.runStage(StageItalicAndBold, text, func(text string) string {
		p := &gfmEmphasis{c: c}
		if p.test(text) {
			text = p.run(text)
		}
		return text
	})
}

// ---- block quotes

const blockQuoteBase = `
        (                           # Wrap whole match in \1
          (
            ^[ \t]*>%s[ \t]?        # '>' at the start of a line
              .+\n                  # rest of the first line
            (.+\n)*                 # subsequent consecutive lines
          )+
        )
    `

var (
	blockQuoteRe        = rx(strings.Replace(blockQuoteBase, "%s", "", 1), reM|reX)
	blockQuoteReSpoiler = rx(strings.Replace(blockQuoteBase, "%s", `[ \t]*?!?`, 1), reM|reX)
	bqOneLevelRe        = rx(`^[ \t]*>[ \t]?`, reM)
	bqOneLevelReSpoiler = rx(`^[ \t]*>[ \t]*?![ \t]?`, reM)
	bqAllLinesSpoilers  = rx(`\A(?:^[ \t]*>[ \t]*?!.*[\n\r]*)+\Z`, reM)
	htmlPreBlockRe      = rx(`(\s*<pre>.+?</pre>)`, reS)
)

func (c *Converter) blockQuoteSub(m *pyre.Match) string {
	bq := m.Group(1)
	spoiler := c.has("spoiler") && bqAllLinesSpoilers.MatchString(bq) != nil
	if spoiler {
		bq = bqOneLevelReSpoiler.ReplaceLiteral(bq, "")
	} else {
		bq = bqOneLevelRe.ReplaceLiteral(bq, "")
	}
	bq = wsOnlyLineRe.ReplaceLiteral(bq, "")
	bq = c.runBlockGamut(bq)
	bq = rx(`(?m)^`, 0).ReplaceLiteral(bq, "  ")
	bq = htmlPreBlockRe.Sub(bq, func(m *pyre.Match) string {
		return rx(`(?m)^  `, 0).ReplaceLiteral(m.Group(1), "")
	})
	if spoiler {
		return "<blockquote class=\"spoiler\">\n" + bq + "\n</blockquote>\n\n"
	}
	return "<blockquote>\n" + bq + "\n</blockquote>\n\n"
}

func (c *Converter) doBlockQuotes(text string) string {
	return c.runStage(StageBlockQuotes, text, func(text string) string {
		if !strings.Contains(text, ">") {
			return text
		}
		if c.has("spoiler") {
			return blockQuoteReSpoiler.Sub(text, c.blockQuoteSub)
		}
		return blockQuoteRe.Sub(text, c.blockQuoteSub)
	})
}

// ---- paragraphs

var paragraphSplitRe = rx(`\n{2,}`, 0)

func (c *Converter) formParagraphs(text string) string {
	return c.runStage(StageParagraphs, text, func(text string) string {
		text = strings.Trim(text, "\n")
		var grafs []string
		for _, graf := range paragraphSplitRe.Split(text, 0) {
			if html, ok := c.htmlBlocks[graf]; ok {
				grafs = append(grafs, html)
				continue
			}
			cuddled := ""
			if c.has("cuddled-lists") {
				li := listItemRe.Search(graf+"\n", 0)
				if li != nil && utf8.RuneCountInString(li.Group(2)) <= 3 {
					next, hasNext := li.Named("next_marker")
					marker, _ := li.Named("marker")
					if (hasNext && next != "" && marker[len(marker)-1] == next[len(next)-1]) || !hasNext {
						start := li.Start(0)
						cuddled = strings.TrimRight(c.doLists(graf[start:]), "\n")
						if rx(`^<(?:ul|ol).*?>`, 0).MatchString(cuddled) != nil {
							graf = graf[:start]
						} else {
							graf = cuddled
							cuddled = ""
						}
					}
				}
			}
			graf = c.runSpanGamut(graf)
			grafs = append(grafs, "<p"+c.htmlClassStrFromTag("p")+">"+strings.TrimLeft(graf, " \t")+"</p>")
			if cuddled != "" {
				grafs = append(grafs, cuddled)
			}
		}
		return strings.Join(grafs, "\n\n")
	})
}

// ---- footnotes

func (c *Converter) addFootnotes(text string) string {
	if c.footnotes.len() == 0 {
		return text
	}
	footer := []string{`<div class="footnotes">`, "<hr" + c.emptyElementSuffix, "<ol>"}
	if c.footnoteTitle == "" {
		c.footnoteTitle = "Jump back to footnote %d in the text."
	}
	if c.footnoteReturnSymbol == "" {
		c.footnoteReturnSymbol = "&#8617;"
	}
	position := map[string]int{}
	for i, k := range c.footnotes.keys {
		position[k] = i
	}
	for _, id := range c.footnoteIDs {
		if _, ok := position[id]; !ok {
			raise("ValueError", "%s is not in list", pyunicode.Repr(id))
		}
	}
	sort.SliceStable(c.footnoteIDs, func(a, b int) bool { return position[c.footnoteIDs[a]] < position[c.footnoteIDs[b]] })
	// Footnotes referenced only from other footnotes are appended while the list is walked.
	for i := 0; i < len(c.footnoteIDs); i++ {
		id := c.footnoteIDs[i]
		if i != 0 {
			footer = append(footer, "")
		}
		footer = append(footer, `<li id="fn-`+id+`">`)
		body, _ := c.footnotes.get(id)
		footer = append(footer, c.doFootnoteMarker(c.runBlockGamut(body)))
		backlink := c.footnoteBacklink(id, i+1)
		if last := footer[len(footer)-1]; strings.HasSuffix(last, "</p>") {
			footer[len(footer)-1] = last[:len(last)-len("</p>")] + "&#160;" + backlink + "</p>"
		} else {
			footer = append(footer, "\n<p>"+backlink+"</p>")
		}
		footer = append(footer, "</li>")
	}
	footer = append(footer, "</ol>", "</div>")
	return text + "\n\n" + strings.Join(footer, "\n")
}

// footnoteBacklink formats the back link with the printf-style title and symbol; a template that
// does not fit the id and number (a TypeError in Python) falls back to the default title.
func (c *Converter) footnoteBacklink(id string, n int) (link string) {
	format := `<a href="#fnref-%s" class="footnoteBackLink" title="` + c.footnoteTitle + `">` + c.footnoteReturnSymbol + `</a>`
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok && e.Type == "TypeError" {
				debugf("Footnote error. `footnote_title` must include parameter. Using defaults.")
				link = `<a href="#fnref-` + id + `" class="footnoteBackLink" title="Jump back to footnote ` +
					strconv.Itoa(n) + ` in the text.">&#8617;</a>`
				return
			}
			panic(r)
		}
	}()
	return pyFormat(format, id, n)
}

var (
	nakedLtRe = rx(`<(?![a-z/?\$!])`, reI)
	nakedGtRe = rx(`(?<![a-z0-9?!/'"-])>`, reI)
)

func (c *Converter) encodeAmpsAndAngles(text string) string {
	text = ampersandRe.ReplaceLiteral(text, "&amp;")
	text = escapedAmpersandRe.MustSubTemplate(text, `&amp;\1`)
	text = nakedLtRe.ReplaceLiteral(text, "&lt;")
	text = nakedGtRe.ReplaceLiteral(text, "&gt;")
	return text
}

var incompleteTagsRe = rx(`\\*<(!--|/?\w+?(?!\w)\s*?.*?(?:[\s/]+?|$))`, 0)

func (c *Converter) encodeIncompleteTags(text string) string {
	if c.safeMode != "replace" && c.safeMode != "escape" {
		return text
	}
	if c.isAutoLink(text) {
		return text
	}
	return incompleteTagsRe.Sub(text, func(m *pyre.Match) string {
		t := m.Text()
		if isUnescapedRe.MatchString(t) == nil {
			t = strings.ReplaceAll(t, `\<`, "&lt;")
		}
		return strings.ReplaceAll(t, "<", "&lt;")
	})
}

func (c *Converter) encodeBackslashEscapes(text string) string {
	for _, ch := range append([]string(nil), c.escapeTable.keys...) {
		text = strings.ReplaceAll(text, `\`+ch, c.escapeTable.m[ch])
	}
	return text
}

var (
	autoLinkRe      = rx(`<((https?|ftp):[^\'">\s]+)>`, reI)
	autoEmailLinkRe = rx(`
          <
           (?:mailto:)?
          (
              [-.\w]+
              \@
              [-\w]+(\.[-\w]+)*\.[a-z]+
          )
          >
        `, reI|reX|reU)
)

func (c *Converter) doAutoLinks(text string) string {
	text = autoLinkRe.Sub(text, func(m *pyre.Match) string {
		g1 := m.Group(1)
		return `<a href="` + c.protectURL(g1) + `">` + htmlEscapeURL(g1, false) + "</a>"
	})
	text = autoEmailLinkRe.Sub(text, func(m *pyre.Match) string {
		return c.encodeEmailAddress(c.unescapeSpecialChars(m.Group(1)))
	})
	return text
}

func (c *Converter) encodeEmailAddress(addr string) string {
	var chars []string
	for _, ch := range "mailto:" + addr {
		chars = append(chars, xmlEncodeEmailCharAtRandom(ch))
	}
	return `<a href="` + strings.Join(chars, "") + `">` + strings.Join(chars[7:], "") + "</a>"
}

var hashTokenRe = rx(`md5-[0-9a-f]{32}`, 0)

func (c *Converter) unescapeSpecialChars(text string) string {
	replacements := map[string]string{}
	setDefault := func(hash, ch string) {
		if _, ok := replacements[hash]; !ok {
			replacements[hash] = ch
		}
	}
	for _, ch := range c.escapeTable.keys {
		setDefault(c.escapeTable.m[ch], ch)
	}
	for _, ch := range c.codeTable.keys {
		setDefault(c.codeTable.m[ch], ch)
	}
	for hash, html := range c.htmlBlocks {
		setDefault(hash, html)
	}
	for {
		orig := text
		text = hashTokenRe.Sub(text, func(m *pyre.Match) string {
			if r, ok := replacements[m.Text()]; ok {
				return r
			}
			return m.Text()
		})
		if text == orig {
			return text
		}
	}
}

func (c *Converter) hashSpan(text string, table *orderedMap) string {
	key := hashText(text)
	if table != nil {
		table.set(key, text)
	} else {
		c.htmlSpans.set(key, text)
	}
	return key
}

// uniformOutdent removes the smallest common leading indentation of the non-empty lines and
// returns that indentation and the outdented text.
func uniformOutdent(text, minOutdent string, hasMin bool, maxOutdent string, hasMax bool) (string, string) {
	lines := pyunicode.SplitLines(text, false)
	ws := make([]string, len(lines))
	present := make([]bool, len(lines))
	var nonEmpty []string
	for i, line := range lines {
		if line != "" {
			ws[i] = rx(`^[ \t]*`, 0).MatchString(line).Text()
			present[i] = true
			nonEmpty = append(nonEmpty, ws[i])
		}
	}
	if len(nonEmpty) == 0 {
		return "", text
	}
	outdent := nonEmpty[0]
	for _, w := range nonEmpty {
		if w < outdent {
			outdent = w
		}
	}
	if hasMin {
		found := false
		for _, w := range nonEmpty {
			if w >= minOutdent && (!found || w < outdent) {
				outdent, found = w, true
			}
		}
		if !found {
			outdent = minOutdent
		}
	}
	if hasMax && maxOutdent < outdent {
		outdent = maxOutdent
	}
	var out strings.Builder
	keep := pyunicode.SplitLines(text, true)
	for i, line := range keep {
		if i >= len(lines) {
			break
		}
		switch {
		case strings.HasPrefix(line, outdent):
			out.WriteString(strings.Replace(line, outdent, "", 1))
		case present[i] && ws[i] < outdent:
			out.WriteString(strings.Replace(line, ws[i], "", 1))
		default:
			out.WriteString(line)
		}
	}
	return outdent, out.String()
}

// uniformIndent indents every line of text by indent.
func uniformIndent(text, indent string, includeEmptyLines, indentEmptyLines bool) string {
	var b strings.Builder
	for _, line := range pyunicode.SplitLines(text, true) {
		switch {
		case pyunicode.TrimSpace(line) != "" || indentEmptyLines:
			b.WriteString(indent + line)
		case includeEmptyLines:
			b.WriteString(line)
		}
	}
	return b.String()
}

// matchOverlapsSubstr reports whether the match starts or ends inside an occurrence of substr.
func matchOverlapsSubstr(text string, m *pyre.Match, substr string) bool {
	for _, inst := range rx(pyre.Escape(substr), 0).FindAll(text) {
		start, end := inst.Start(0), inst.End(0)
		if start <= m.Start(0) && m.Start(0) <= end {
			return true
		}
		if start <= m.End(0) && m.End(0) <= end {
			return true
		}
	}
	return false
}
