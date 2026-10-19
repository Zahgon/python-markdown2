// Go port of python-markdown2. Copyright (c) 2007-2008 ActiveState Software Inc. (python-markdown2 test suite)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The case directories under test/ hold NAME.text inputs with the expected NAME.html, and
// optionally NAME.opts (keyword arguments), NAME.tags, NAME.toc_html and NAME.metadata.
var caseDirs = []string{"tm-cases", "markdowntest-cases", "php-markdown-cases", "php-markdown-extra-cases"}

// skippedTags are the tags of cases the suite does not run: known failures, and cases that need
// the optional Python packages (pygments, wavedrom, latex2mathml, emoji) this port does not use.
var skippedTags = map[string]bool{"knownfailure": true, "pygments": true, "wavedrom": true, "latex2mathml": true, "emoji": true}

var (
	emailLinkRe = regexp.MustCompile(`<a href="(.*?&#.*?)">(.*?)</a>`)
	charRefRe   = regexp.MustCompile(`&#(x[0-9A-Fa-f]{2,3}|[0-9]{2,3});`)
)

// normHTML undoes the random entity encoding of e-mail links and normalises line endings.
func normHTML(html string) string {
	decode := func(s string) string {
		return charRefRe.ReplaceAllStringFunc(s, func(ref string) string {
			body := ref[2 : len(ref)-1]
			var n int64
			if body[0] == 'x' {
				n, _ = strconv.ParseInt(body[1:], 16, 32)
			} else {
				n, _ = strconv.ParseInt(body, 10, 32)
			}
			return string(rune(n))
		})
	}
	html = emailLinkRe.ReplaceAllStringFunc(html, func(m string) string {
		sub := emailLinkRe.FindStringSubmatch(m)
		return `<a href="` + decode(sub[1]) + `">` + decode(sub[2]) + `</a>`
	})
	return strings.ReplaceAll(html, "\r\n", "\n")
}

func readTags(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var tags []string
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		tags = append(tags, strings.Fields(line)...)
	}
	return tags
}

// pepLink is the callable of the PEP link pattern in the link_patterns cases.
func pepLink(m LinkMatch) string {
	n, _ := strconv.Atoi(m.Group(1))
	return fmt.Sprintf("http://www.python.org/dev/peps/pep-%04d/", n)
}

// optionsFromOpts turns the keyword arguments of an opts file into Options.
func optionsFromOpts(t *testing.T, kwargs map[string]any) Options {
	var opts Options
	for k, v := range kwargs {
		switch k {
		case "extras":
			opts.Extras = map[string]any{}
			switch e := v.(type) {
			case []any:
				for _, name := range e {
					opts.Extras[name.(string)] = nil
				}
			case map[string]any:
				opts.Extras = e
			}
		case "safe_mode":
			if b, ok := v.(bool); ok {
				if b {
					opts.SafeMode = "replace"
				}
			} else {
				opts.SafeMode = v.(string)
			}
		case "html4tags":
			opts.HTML4Tags = v.(bool)
		case "tab_width":
			opts.TabWidth = v.(int)
		case "use_file_vars":
			opts.UseFileVars = v.(bool)
		case "footnote_title":
			opts.FootnoteTitle = v.(string)
		case "footnote_return_symbol":
			opts.FootnoteReturnSymbol = v.(string)
		case "link_patterns":
			for _, p := range v.([]any) {
				pair := p.([]any)
				re := pair[0].(compiledPattern)
				lp := LinkPattern{Pattern: "/" + re.pattern + "/" + re.flags}
				switch href := pair[1].(type) {
				case string:
					lp.Href = href
				case lambdaValue:
					if !strings.Contains(href.source, "pep-%04d") {
						t.Fatalf("unsupported link pattern callable %q", href.source)
					}
					lp.Func = pepLink
				}
				opts.LinkPatterns = append(opts.LinkPatterns, lp)
			}
		default:
			t.Fatalf("unknown option %q", k)
		}
	}
	return opts
}

func TestCases(t *testing.T) {
	for _, dir := range caseDirs {
		paths, err := filepath.Glob(filepath.Join("test", dir, "*.text"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("no cases in test/%s", dir)
		}
		for _, textPath := range paths {
			base := strings.TrimSuffix(textPath, ".text")
			name := dir + "/" + filepath.Base(base)
			t.Run(name, func(t *testing.T) {
				for _, tag := range readTags(base + ".tags") {
					if skippedTags[tag] {
						t.Skipf("tagged %q", tag)
					}
				}
				text, err := os.ReadFile(textPath)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(base + ".html")
				if err != nil {
					t.Fatal(err)
				}
				var opts Options
				if data, err := os.ReadFile(base + ".opts"); err == nil {
					lit, err := parsePyLiteral(string(data))
					if err != nil {
						t.Fatalf("%s.opts: %v", base, err)
					}
					opts = optionsFromOpts(t, lit.(map[string]any))
				}
				res, err := Markdown(string(text), opts)
				if err != nil {
					t.Fatalf("Markdown(%s) failed: %v", textPath, err)
				}
				if got, exp := normHTML(res.HTML), normHTML(string(want)); got != exp {
					t.Errorf("Markdown(%s) with %s.opts:\n---- got ----\n%s\n---- want (%s.html) ----\n%s", textPath, base, got, base, exp)
				}
				if tocWant, err := os.ReadFile(base + ".toc_html"); err == nil {
					if res.TOCHTML == nil {
						t.Errorf("Markdown(%s): no TOCHTML, want the contents of %s.toc_html", textPath, base)
					} else if normHTML(*res.TOCHTML) != normHTML(string(tocWant)) {
						t.Errorf("Markdown(%s).TOCHTML:\n---- got ----\n%s\n---- want (%s.toc_html) ----\n%s", textPath, *res.TOCHTML, base, tocWant)
					}
				}
				if metaWant, err := os.ReadFile(base + ".metadata"); err == nil {
					var want map[string]any
					if err := json.Unmarshal(metaWant, &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(res.Metadata, want) {
						t.Errorf("Markdown(%s).Metadata = %#v, want %#v (%s.metadata)", textPath, res.Metadata, want, base)
					}
				}
			})
		}
	}
}

func assertMarkdown(t *testing.T, text, want string, opts Options) {
	t.Helper()
	res, err := Markdown(text, opts)
	if err != nil {
		t.Fatalf("Markdown(%q) failed: %v", text, err)
	}
	if normHTML(res.HTML) != normHTML(want) {
		t.Errorf("Markdown(%q) = %q, want %q", text, res.HTML, want)
	}
}

func convert(t *testing.T, c *Converter, text string) *Result {
	t.Helper()
	res, err := c.Convert(text)
	if err != nil {
		t.Fatalf("Convert(%q) failed: %v", text, err)
	}
	return res
}

func TestHeaderIDsFromFileVars(t *testing.T) {
	c, err := New(Options{UseFileVars: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ option, id string }{{"", "heading"}, {"=chapter", "chapter-heading"}} {
		text := "<!-- -*- markdown-extras: header-ids" + tc.option + " -*- -->\n# Heading\n"
		want := `<h1 id="` + tc.id + `">Heading</h1>`
		if html := convert(t, c, text).HTML; !strings.Contains(html, want) {
			t.Errorf("Convert(%q) = %q, want it to contain %q", text, html, want)
		}
		if html := convert(t, c, "# Heading\n").HTML; html != "<h1>Heading</h1>\n" {
			t.Errorf("Convert(%q) after a file-vars document = %q, want %q", "# Heading\n", html, "<h1>Heading</h1>\n")
		}
	}
}

func TestTOCFromFileVars(t *testing.T) {
	c, err := New(Options{UseFileVars: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"toc", "header-ids, toc"} {
		text := "<!-- -*- markdown-extras: " + option + " -*- -->\n# Heading\n"
		res := convert(t, c, text)
		if !strings.Contains(res.HTML, `<h1 id="heading">Heading</h1>`) {
			t.Errorf("Convert(%q) = %q, want a header with id heading", text, res.HTML)
		}
		if res.TOCHTML == nil || !strings.Contains(*res.TOCHTML, `<a href="#heading">Heading</a>`) {
			t.Errorf("Convert(%q).TOCHTML = %v, want a link to #heading", text, res.TOCHTML)
		}
		if html := convert(t, c, "# Heading\n").HTML; html != "<h1>Heading</h1>\n" {
			t.Errorf("Convert(%q) after a file-vars document = %q, want %q", "# Heading\n", html, "<h1>Heading</h1>\n")
		}
	}
}

func TestFootnoteNumbersResetBetweenConversions(t *testing.T) {
	c, err := New(Options{Extras: map[string]any{"footnotes": nil}})
	if err != nil {
		t.Fatal(err)
	}
	convert(t, c, "First[^first], second[^second].\n\n[^first]: First note.\n[^second]: Second note.")
	html := convert(t, c, "Second[^second].\n\n[^second]: Second note.").HTML
	for _, want := range []string{`<a href="#fn-second">1</a>`, `<li id="fn-second">`} {
		if !strings.Contains(html, want) {
			t.Errorf("second conversion = %q, want it to contain %q", html, want)
		}
	}
}

func TestManyDistinctCodeSpans(t *testing.T) {
	var src, want []string
	for i := 0; i < 1000; i++ {
		src = append(src, fmt.Sprintf("`value_%d`", i))
		want = append(want, fmt.Sprintf("<p><code>value_%d</code></p>", i))
	}
	assertMarkdown(t, strings.Join(src, "\n\n"), strings.Join(want, "\n\n")+"\n", Options{})
}

func newReset(t *testing.T) *Converter {
	t.Helper()
	c, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	c.reset()
	return c
}

func TestUnescapeNestedTokens(t *testing.T) {
	inner := `\1\g<0>\\ *`
	middle := "<code>" + hashText(inner) + "</code>"
	outer := "<div>" + hashText(middle) + "</div>"
	for _, order := range [][2]string{{inner, middle}, {middle, inner}} {
		c := newReset(t)
		c.codeTable = newOrderedMap()
		for _, s := range order {
			c.codeTable.set(s, hashText(s))
		}
		c.htmlBlocks = map[string]string{hashText(outer): outer}
		if got, want := c.unescapeSpecialChars(hashText(outer)), "<div><code>"+inner+"</code></div>"; got != want {
			t.Errorf("unescapeSpecialChars(outer) = %q, want %q", got, want)
		}
	}
}

func TestUnescapeSpecialCharsInsideHTML(t *testing.T) {
	c := newReset(t)
	star, _ := c.escapeTable.get("*")
	bs, _ := c.escapeTable.get(`\`)
	html := "<div>" + star + " " + bs + "</div>"
	token := hashText(html)
	c.htmlBlocks[token] = html
	if got, want := c.unescapeSpecialChars(token), `<div>* \</div>`; got != want {
		t.Errorf("unescapeSpecialChars = %q, want %q", got, want)
	}
}

func TestUnescapeLeavesUnknownTokensAndPlainText(t *testing.T) {
	c := newReset(t)
	text := `plain \ text <div>md5-` + strings.Repeat("0", 32) + "</div>"
	if got := c.unescapeSpecialChars(text); got != text {
		t.Errorf("unescapeSpecialChars(%q) = %q, want it unchanged", text, got)
	}
}

func TestUnescapeRepeatedTokens(t *testing.T) {
	c := newReset(t)
	value := `\g<0> * literal`
	token := hashText(value)
	c.codeTable.set(value, token)
	if got, want := c.unescapeSpecialChars(token+" "+token), value+" "+value; got != want {
		t.Errorf("unescapeSpecialChars = %q, want %q", got, want)
	}
}

func TestUnescapeDuplicateTokenPriority(t *testing.T) {
	c := newReset(t)
	token, _ := c.escapeTable.get("*")
	c.codeTable.set("code value", token)
	c.htmlBlocks[token] = "<div>HTML value</div>"
	if got := c.unescapeSpecialChars(token); got != "*" {
		t.Errorf("unescapeSpecialChars = %q, want %q", got, "*")
	}
}

func TestSlowHR(t *testing.T) {
	text := "* * *\n\nThis on *almost* looks like an hr, except for the trailing '+'. In older\n" +
		"versions of markdown2.py this was pathologically slow:\n\n- - - - - - - - - - - - - - - - - - - - - - - - - +\n"
	want := "<hr />\n\n<p>This on <em>almost</em> looks like an hr, except for the trailing '+'. In older\n" +
		"versions of markdown2.py this was pathologically slow:</p>\n\n<p>- - - - - - - - - - - - - - - - - - - - - - - - - +</p>\n"
	start := time.Now()
	assertMarkdown(t, text, want, Options{})
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("converting the slow-hr text took %v, want under 30s", d)
	}
}

func TestDirectCases(t *testing.T) {
	cases := []struct {
		name, text, want string
		opts             Options
	}{
		{"code in strong", "**look at `this code` call**", "<p><strong>look at <code>this code</code> call</strong></p>\n", Options{}},
		{"starter pre", "    #!/usr/bin/python\n    print \"hi\"", "<pre><code>#!/usr/bin/python\nprint \"hi\"\n</code></pre>\n", Options{}},
		{"pre", "some starter text\n\n    #!/usr/bin/python\n    print \"hi\"",
			"<p>some starter text</p>\n\n<pre><code>#!/usr/bin/python\nprint \"hi\"\n</code></pre>\n", Options{}},
		{"russian", "## \\u043b\\u0449", "<h2>\\u043b\\u0449</h2>\n", Options{}},
		{"style block", "<style>body { margin-left: 1.25cm; }</style>", "<style>body { margin-left: 1.25cm; }</style>\n", Options{}},
		{"head block", `<head><meta charset="UTF-8"></head>`, `<head><meta charset="UTF-8"></head>` + "\n", Options{}},
		{"breaks then break-on-newline", "a\nb", "<p>a<br />\nb</p>\n", Options{Extras: map[string]any{"breaks": nil, "break-on-newline": nil}}},
		{"break-on-newline alone", "a\nb", "<p>a<br />\nb</p>\n", Options{Extras: map[string]any{"break-on-newline": nil}}},
		{"breaks alone", "a\nb", "<p>a\nb</p>\n", Options{Extras: map[string]any{"breaks": nil}}},
		{"breaks dict kept", "a\\\nb", "<p>a<br />\nb</p>\n",
			Options{Extras: map[string]any{"breaks": map[string]any{"on_backslash": true}, "break-on-newline": nil}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertMarkdown(t, tc.text, tc.want, tc.opts) })
	}
}

func TestMetadata(t *testing.T) {
	cases := []struct {
		name, text, html string
		meta             map[string]any
	}{
		{"no blank line", "\tsome indented code", "<pre><code>some indented code\n</code></pre>\n", map[string]any{}},
		{"leading hr without closing fence", "---\n# My Document\n", "<hr />\n\n<h1>My Document</h1>\n", map[string]any{}},
		{"fenced front matter", "---\ntitle: Hi\n---\n# Body\n", "<h1>Body</h1>\n", map[string]any{"title": "Hi"}},
		{"without fence", "title: Hello\nauthor: Me\n\n# Body\n", "<h1>Body</h1>\n", map[string]any{"title": "Hello", "author": "Me"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Markdown(tc.text, Options{Extras: map[string]any{"metadata": nil}})
			if err != nil {
				t.Fatal(err)
			}
			if res.HTML != tc.html {
				t.Errorf("Markdown(%q) = %q, want %q", tc.text, res.HTML, tc.html)
			}
			if !reflect.DeepEqual(res.Metadata, tc.meta) {
				t.Errorf("Markdown(%q).Metadata = %#v, want %#v", tc.text, res.Metadata, tc.meta)
			}
		})
	}
}

func TestTOCWithPersistentConverter(t *testing.T) {
	c, err := New(Options{Extras: map[string]any{"toc": nil}})
	if err != nil {
		t.Fatal(err)
	}
	text := "\n# Header 1\n## Header 1.1\n## Header 1.2\n### Header 1.3\n# Header 2\n## Header 2.1\n        "
	want := "<ul>\n  <li><a href=\"#header-1\">Header 1</a>\n  <ul>\n    <li><a href=\"#header-11\">Header 1.1</a></li>\n" +
		"    <li><a href=\"#header-12\">Header 1.2</a>\n    <ul>\n      <li><a href=\"#header-13\">Header 1.3</a></li>\n" +
		"    </ul></li>\n  </ul></li>\n  <li><a href=\"#header-2\">Header 2</a>\n  <ul>\n" +
		"    <li><a href=\"#header-21\">Header 2.1</a></li>\n  </ul></li>\n</ul>\n"
	for i := 0; i < 2; i++ {
		if got := convert(t, c, text).TOCHTML; got == nil || *got != want {
			t.Errorf("conversion %d: TOCHTML = %v, want %q", i+1, got, want)
		}
	}
	text = "\n# I'm new html\n## I don't have to be long, just different\n"
	want = "<ul>\n  <li><a href=\"#im-new-html\">I'm new html</a>\n  <ul>\n" +
		"    <li><a href=\"#i-dont-have-to-be-long-just-different\">I don't have to be long, just different</a></li>\n  </ul></li>\n</ul>\n"
	if got := convert(t, c, text).TOCHTML; got == nil || *got != want {
		t.Errorf("new document: TOCHTML = %v, want %q", got, want)
	}
}

// TestAPI covers the package-level examples (the Python api.doctests).
func TestAPI(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty")
	}
	res, err := Markdown("*boo*", Options{})
	if err != nil || res.HTML != "<p><em>boo</em></p>\n" {
		t.Errorf(`Markdown("*boo*") = %v, %v; want "<p><em>boo</em></p>\n"`, res, err)
	}
	for name, newConverter := range map[string]func(Options) (*Converter, error){"New": New, "NewWithExtras": NewWithExtras} {
		c, err := newConverter(Options{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if html := convert(t, c, "*boo*").HTML; html != "<p><em>boo</em></p>\n" {
			t.Errorf("%s().Convert(%q) = %q, want %q", name, "*boo*", html, "<p><em>boo</em></p>\n")
		}
	}
}

// redosCases are inputs that once made the converter take exponential time.
var redosCases = []struct {
	name   string
	text   string
	extras []string
}{
	{"pull_387_example_1", "[#a" + strings.Repeat(" ", 3456), nil},
	{"pull_387_example_2", "```" + strings.Repeat("\n", 3456), nil},
	{"pull_387_example_3", "-*-" + strings.Repeat(" ", 3456), nil},
	{"pull_402", strings.Repeat(" ", 100_000) + "$", nil},
	{"issue493", "**_" + strings.Repeat("*_", 38730*10) + "\x00", nil},
	{"issue_633", strings.Repeat(`<p m="1"`, 2500) + strings.Repeat(" ", 5000) + "</div", nil},
	{"issue_668", "a_b **x***y* c_d", []string{"code-friendly"}},
	{"issue_707_unclosed_tags", strings.Repeat(`<p m="1"`, 15000), nil},
	{"issue_707_namespaced_attrs", "x <p" + strings.Repeat(" a:b=1", 40), nil},
	{"issue_707_spaced_attrs", "x <p" + strings.Repeat("  a=1", 40), nil},
	{"issue_metadata_redos", "---\nkey: -" + strings.Repeat(" ", 32000) + "\n---\n\n# hello\n", []string{"metadata"}},
	{"issue_metadata_list_redos", "---\nitems:\n  - " + strings.Repeat(" ", 32000) + "\n---\n\n# hello\n", []string{"metadata"}},
}

func TestReDoS(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	for _, tc := range redosCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{}
			if tc.extras != nil {
				opts.Extras = map[string]any{}
				for _, e := range tc.extras {
					opts.Extras[e] = nil
				}
			}
			done := make(chan error, 1)
			go func() {
				_, err := Markdown(tc.text, opts)
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Markdown(%s) failed: %v", tc.name, err)
				}
			case <-time.After(60 * time.Second):
				t.Errorf("Markdown(%s) took more than 60s", tc.name)
			}
		})
	}
}
