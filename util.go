// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package markdown2

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"strings"
	"unicode/utf8"

	"github.com/trentm/python-markdown2/internal/pyre"
	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// Regex flag shorthands.
const (
	reI = pyre.IGNORECASE
	reM = pyre.MULTILINE
	reS = pyre.DOTALL
	reX = pyre.VERBOSE
	reU = pyre.UNICODE
)

// rx compiles one of the library's own patterns (cached by pyre).
func rx(pattern string, flags pyre.Flags) *pyre.Regexp { return pyre.MustCompile(pattern, flags) }

// secretSalt makes the placeholder hashes unguessable from the input text; it is drawn once per
// process from the runtime's randomly seeded generator.
var secretSalt = func() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[:8], rand.Uint64())
	binary.LittleEndian.PutUint64(b[8:], rand.Uint64())
	return b
}()

// hashText returns the placeholder that stands for s while the text is processed.
func hashText(s string) string {
	if !utf8.ValidString(s) {
		raise("UnicodeEncodeError", "'utf-8' codec can't encode characters: surrogates not allowed")
	}
	h := sha256.New()
	h.Write(secretSalt)
	h.Write([]byte(s))
	return "md5-" + hex.EncodeToString(h.Sum(nil))[32:]
}

// orderedMap is a string map that remembers insertion order (Python dict semantics).
type orderedMap struct {
	keys []string
	m    map[string]string
}

func newOrderedMap() *orderedMap { return &orderedMap{m: map[string]string{}} }

func (o *orderedMap) set(k, v string) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *orderedMap) get(k string) (string, bool) {
	v, ok := o.m[k]
	return v, ok
}

func (o *orderedMap) clone() *orderedMap {
	c := &orderedMap{keys: append([]string(nil), o.keys...), m: make(map[string]string, len(o.m))}
	for k, v := range o.m {
		c.m[k] = v
	}
	return c
}

func (o *orderedMap) len() int { return len(o.keys) }

// runeIndex converts a code point count from the start of s into a byte offset.
func runeOffset(s string, n int) int {
	if n <= 0 {
		return 0
	}
	i := 0
	for i < len(s) && n > 0 {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n--
	}
	return i
}

// headRunes is s[:n] in code points; tailRunes is s[-n:].
func headRunes(s string, n int) string { return s[:runeOffset(s, n)] }

func tailRunes(s string, n int) string {
	c := utf8.RuneCountInString(s)
	if c <= n {
		return s
	}
	return s[runeOffset(s, c-n):]
}

// dropFirstRune is s[1:] in code points.
func dropFirstRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeRuneInString(s)
	return s[size:]
}

// dedentLines removes the common leading indentation of lines in place (the "dedent" recipe).
func dedentLines(lines []string, tabsize int, skipFirstLine bool) []string {
	margin := -1
	for i, line := range lines {
		if i == 0 && skipFirstLine {
			continue
		}
		indent := 0
		blank := true
		for _, ch := range line {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += tabsize - indent%tabsize
			} else if ch == '\r' || ch == '\n' {
				continue
			} else {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		if margin < 0 || indent < margin {
			margin = indent
		}
	}
	if margin <= 0 {
		return lines
	}
	for i, line := range lines {
		if i == 0 && skipFirstLine {
			continue
		}
		removed := 0
		done := false
		for j, ch := range line {
			switch {
			case ch == ' ':
				removed++
			case ch == '\t':
				removed += tabsize - removed%tabsize
			case ch == '\r' || ch == '\n':
				lines[i] = line[j:]
				done = true
			default:
				raise("ValueError", "unexpected non-whitespace char %s in line %s while removing %d-space margin",
					pyunicode.Repr(string(ch)), pyunicode.Repr(line), margin)
			}
			if done {
				break
			}
			if removed == margin {
				lines[i] = line[j+1:]
				done = true
				break
			}
			if removed > margin {
				lines[i] = strings.Repeat(" ", removed-margin) + line[j+1:]
				done = true
				break
			}
		}
		if !done && removed > 0 {
			lines[i] = line[runeOffset(line, removed):]
		}
	}
	return lines
}

// dedent is textwrap.dedent without tab expansion.
func dedent(text string, tabsize int, skipFirstLine bool) string {
	lines := pyunicode.SplitLines(text, true)
	return strings.Join(dedentLines(lines, tabsize, skipFirstLine), "")
}

// slugify turns header text into an id: compatibility-decomposed, lowercased, non-word
// characters removed and runs of spaces and hyphens joined by one hyphen.
func slugify(value string) string {
	value = strings.ToValidUTF8(pyunicode.NFKD(value), "")
	value = pyunicode.Lower(pyunicode.TrimSpace(rx(`[^\w\s-]`, 0).ReplaceLiteral(value, "")))
	return rx(`[-\s]+`, 0).ReplaceLiteral(value, "-")
}

// regexFromEncodedPattern compiles a link pattern: "/pattern/flags" or a literal string.
func regexFromEncodedPattern(s string) (*pyre.Regexp, error) {
	if strings.HasPrefix(s, "/") && strings.LastIndex(s, "/") != 0 {
		idx := strings.LastIndex(s, "/")
		pattern, flagsStr := s[1:idx], s[idx+1:]
		var flags pyre.Flags
		for _, ch := range flagsStr {
			switch ch {
			case 'i':
				flags |= pyre.IGNORECASE
			case 'l':
				flags |= pyre.LOCALE
			case 's':
				flags |= pyre.DOTALL
			case 'm':
				flags |= pyre.MULTILINE
			case 'u':
				flags |= pyre.UNICODE
			default:
				return nil, &Error{Type: "ValueError", Message: fmt.Sprintf(
					"unsupported regex flag: '%c' in '%s' (must be one of 'ilsmu')", ch, s)}
			}
		}
		if flags&pyre.LOCALE != 0 {
			return nil, &Error{Type: "ValueError", Message: "cannot use LOCALE flag with a str pattern"}
		}
		re, err := pyre.Compile(pattern, flags)
		if err != nil {
			return nil, &Error{Type: "re.error", Message: err.Error()}
		}
		return re, nil
	}
	return pyre.MustCompile(pyre.Escape(s), 0), nil
}

// xmlEscapeAttr escapes text for a double-quoted attribute (ampersands are left alone).
func xmlEscapeAttr(attr string) string {
	r := strings.NewReplacer(`"`, "&quot;", "<", "&lt;", ">", "&gt;")
	return r.Replace(attr)
}

// htmlEscapeURL escapes a URL for an href or src attribute.
func htmlEscapeURL(attr string, safeMode bool) string {
	escaped := strings.NewReplacer(`"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(attr)
	if safeMode {
		escaped = strings.ReplaceAll(escaped, "'", "&#39;")
	}
	return escaped
}

// xmlEncodeEmailCharAtRandom encodes one character of an e-mail address as a decimal or hex
// character reference, or (about one time in ten, never for @ and _) leaves it as is.
func xmlEncodeEmailCharAtRandom(ch rune) string {
	r := rand.Float64()
	switch {
	case r > 0.9 && ch != '@' && ch != '_':
		return string(ch)
	case r < 0.45:
		return fmt.Sprintf("&#x%x;", ch)
	}
	return fmt.Sprintf("&#%d;", ch)
}

// calculateTOCHTML renders the table of contents entries as nested lists.
func calculateTOCHTML(toc []tocEntry) string {
	var lines []string
	hStack := []int{0}
	indent := func() string { return strings.Repeat("  ", len(hStack)-1) }
	for _, e := range toc {
		switch {
		case e.level > hStack[len(hStack)-1]:
			lines = append(lines, indent()+"<ul>")
			hStack = append(hStack, e.level)
		case e.level == hStack[len(hStack)-1]:
			lines[len(lines)-1] += "</li>"
		default:
			for e.level < hStack[len(hStack)-1] {
				if len(hStack) == 1 || len(lines) == 0 {
					raise("IndexError", "list index out of range")
				}
				hStack = hStack[:len(hStack)-1]
				if !strings.HasSuffix(lines[len(lines)-1], "</li>") {
					lines[len(lines)-1] += "</li>"
				}
				lines = append(lines, indent()+"</ul></li>")
			}
		}
		lines = append(lines, fmt.Sprintf(`%s<li><a href="#%s">%s</a>`, indent(), e.id, e.name))
	}
	for len(hStack) > 1 {
		hStack = hStack[:len(hStack)-1]
		if !strings.HasSuffix(lines[len(lines)-1], "</li>") {
			lines[len(lines)-1] += "</li>"
		}
		lines = append(lines, indent()+"</ul>")
	}
	return strings.Join(lines, "\n") + "\n"
}
