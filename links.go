// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"strings"
	"unicode/utf8"

	"github.com/trentm/python-markdown2/internal/pyre"
	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// linkProcessor turns inline and reference links and images into <a> and <img> tags, and
// footnote references into footnote links.
type linkProcessor struct {
	c       *Converter
	options any
	// mdFileLinks makes it the markdown-file-links extra (only .md links, rewritten to .html).
	mdFileLinks bool
}

func newLinkProcessor(c *Converter, options any) *linkProcessor {
	if !truthy(options) {
		options = map[string]any{}
	}
	return &linkProcessor{c: c, options: options}
}

func (l *linkProcessor) test(text string) bool {
	return strings.Contains(text, "(") || strings.Contains(text, "[")
}

// parseInlineAnchorOrImage parses "(url "title")" starting at start; ok is false when it is not
// a link.
func (l *linkProcessor) parseInlineAnchorOrImage(text string, start int) (url string, title string, hasTitle bool, end int, ok bool) {
	c := l.c
	idx := c.findNonWhitespace(text, start+1)
	if idx == len(text) {
		return "", "", false, 0, false
	}
	endIdx := idx
	angle := text[idx] == '<'
	if angle {
		endIdx = findBalanced(text, endIdx+1, '<', '>')
	}
	endIdx = findBalanced(text, endIdx, '(', ')')
	m := inlineLinkTitle.SearchRange(text, idx, endIdx)
	if m == nil {
		return "", "", false, 0, false
	}
	url = text[idx:m.Start(0)]
	title, hasTitle = m.Named("title")
	if angle {
		url = stripAngleBrackets.MustSubTemplate(url, `\1`)
	}
	if l.mdFileLinks {
		if url == "" || !strings.HasSuffix(url, ".md") {
			return "", "", false, 0, false
		}
		url = strings.TrimSuffix(url, ".md") + ".html"
	}
	return url, title, hasTitle, endIdx, true
}

var shortrefTailRe = rx(`[ ]?(?:\n[ ]*)?(?!\[)`, 0)

func (l *linkProcessor) processLinkShortrefs(text, linkText string, start int) (*pyre.Match, string) {
	if linkText != "" && shortrefTailRe.MatchString(text[start:]) != nil {
		candidate := text[:start] + "[]" + text[start:]
		if m := tailOfReferenceLinkRe.MatchAt(candidate, start); m != nil {
			id, _ := m.Named("id")
			key := pyunicode.Lower(id)
			if key == "" {
				key = pyunicode.Lower(linkText)
			}
			if _, ok := l.c.urls[key]; ok {
				return m, candidate
			}
		}
	}
	return nil, text
}

func (l *linkProcessor) parseRefAnchorOrRefImage(text, linkText string, start int) (string, string, bool, string, bool, int, bool) {
	var m *pyre.Match
	if l.c.has("link-shortrefs") {
		m, text = l.processLinkShortrefs(text, linkText, start)
	}
	if m == nil {
		m = tailOfReferenceLinkRe.MatchAt(text, start)
	}
	if m == nil {
		return text, "", false, "", false, 0, false
	}
	id, _ := m.Named("id")
	linkID := pyunicode.Lower(id)
	if linkID == "" {
		linkID = pyunicode.Lower(linkText)
	}
	url, hasURL := l.c.urls[linkID]
	title, hasTitle := l.c.titles[linkID]
	return text, url, hasURL, title, hasTitle, m.End(0), true
}

func (l *linkProcessor) option(key string, def any) any { return optGet(l.options, key, def) }

func (l *linkProcessor) tagAllowed(tag string) bool {
	tags := l.option("tags", []any{tag})
	switch t := tags.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && s == tag {
				return true
			}
		}
		return false
	case string:
		return strings.Contains(t, tag)
	case map[string]any:
		_, ok := t[tag]
		return ok
	}
	raise("TypeError", "argument of type '%s' is not iterable", typeName(tags))
	return false
}

func (l *linkProcessor) processImage(url, titleAttr, linkText string) (string, int) {
	c := l.c
	result := `<img src="` + c.protectURL(url) + `" alt="` + c.hashSpan(xmlEscapeAttr(linkText), nil) + `"` +
		titleAttr + c.htmlClassStrFromTag("img") + c.emptyElementSuffix
	return result, len(result)
}

func (l *linkProcessor) processAnchor(url, titleAttr, linkText string) (string, int) {
	c := l.c
	var head string
	if c.safeMode != "" && c.safeHref().MatchString(url) == nil {
		head = `<a href="#"` + titleAttr + ">"
	} else {
		head = `<a href="` + c.protectURL(url) + `"` + titleAttr + ">"
	}
	return head + linkText + "</a>", len(head)
}

const maxLinkTextSentinel = 3000

func (l *linkProcessor) run(text string) string {
	c := l.c
	if l.mdFileLinks && c.order > 2*int(StageLinkDefs) && c.order < 2*int(StageLinks) && truthy(l.option("link_defs", true)) {
		for key, url := range c.urls {
			if strings.HasSuffix(url, ".md") {
				c.urls[key] = strings.TrimSuffix(url, ".md") + ".html"
			}
		}
	}
	anchorAllowedPos := 0
	currPos := 0
	for {
		rel := strings.IndexByte(text[min(currPos, len(text)):], '[')
		if rel < 0 {
			break
		}
		startIdx := min(currPos, len(text)) + rel

		// find the matching ']' within the sentinel length (counted in characters)
		depth := 0
		p := startIdx + 1
		found := false
		for n := 1; n < maxLinkTextSentinel && p < len(text); n++ {
			ch := text[p]
			if ch == ']' {
				depth--
				if depth < 0 {
					found = true
					break
				}
			} else if ch == '[' {
				depth++
			}
			_, size := utf8.DecodeRuneInString(text[p:])
			p += size
		}
		if !found {
			currPos = startIdx + 1
			continue
		}
		linkText := text[startIdx+1 : p]

		if c.safeMode != "" {
			linkText = c.hashHTMLSpans(linkText)
			linkText = c.unhashHTMLSpans(linkText, true, false)
		}

		inAutolink := false
		for _, a := range autoLinkRe.FindAll(text) {
			if a.Start(0) < startIdx && startIdx < a.End(0) || a.Start(0) < p && p < a.End(0) {
				inAutolink = true
				break
			}
		}
		if inAutolink {
			currPos = startIdx + 1
			continue
		}

		if c.has("footnotes") && strings.HasPrefix(linkText, "^") {
			normed := rx(`\W`, 0).ReplaceLiteral(linkText[1:], "-")
			if _, ok := c.footnotes.get(normed); ok {
				result := `<sup class="footnote-ref" id="fnref-` + normed + `"><a href="#fn-` + normed + `">` +
					c.footnoteMarker + "-" + normed + "</a></sup>"
				text = text[:startIdx] + result + text[p+1:]
			} else {
				currPos = p + 1
			}
			continue
		}

		p++
		var url, title string
		var hasTitle bool
		var urlEnd int
		if p < len(text) && text[p] == '(' {
			if !truthy(l.option("inline", true)) {
				currPos = startIdx + 1
				continue
			}
			u, t, ht, e, ok := l.parseInlineAnchorOrImage(text, p)
			if !ok {
				currPos = startIdx + 1
				continue
			}
			url, title, hasTitle, urlEnd = u, t, ht, e
		} else {
			if !truthy(l.option("ref", true)) {
				currPos = startIdx + 1
				continue
			}
			t2, u, hasURL, t, ht, e, ok := l.parseRefAnchorOrRefImage(text, linkText, p)
			if !ok {
				currPos = startIdx + 1
				continue
			}
			text = t2
			if !hasURL {
				currPos = p
				continue
			}
			url, title, hasTitle, urlEnd = u, t, ht, e
		}

		titleStr := ""
		if hasTitle && title != "" {
			if c.safeMode != "" {
				title = c.unhashHTMLSpans(title, true, true)
			}
			star, _ := c.escapeTable.get("*")
			under, _ := c.escapeTable.get("_")
			title = strings.ReplaceAll(strings.ReplaceAll(xmlEscapeAttr(title), "*", star), "_", under)
			if c.safeMode != "" {
				title = c.hashSpan(title, nil)
			}
			titleStr = ` title="` + title + `"`
		}

		isImg := startIdx > 0 && text[startIdx-1] == '!'
		var result string
		var skip int
		switch {
		case isImg:
			if !l.tagAllowed("img") {
				currPos = startIdx + 1
				continue
			}
			if linkText != "" && c.safeMode != "" {
				linkText = c.unhashHTMLSpans(linkText, true, true)
			}
			startIdx--
			result, skip = l.processImage(url, titleStr, linkText)
		case startIdx >= anchorAllowedPos:
			if !l.tagAllowed("a") {
				currPos = startIdx + 1
				continue
			}
			result, skip = l.processAnchor(url, titleStr, linkText)
		default:
			currPos = startIdx + 1
			continue
		}
		if c.has("smarty-pants") {
			q, _ := c.escapeTable.get(`"`)
			result = strings.ReplaceAll(result, `"`, q)
		}
		currPos = startIdx + skip
		anchorAllowedPos = startIdx + len(result)
		text = text[:startIdx] + result + text[min(urlEnd, len(text)):]
	}
	return text
}
