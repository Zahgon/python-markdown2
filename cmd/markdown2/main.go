// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

// Command markdown2 converts Markdown files (or standard input) to HTML.
//
//	markdown2 [PATHS...]
//
// See markdown2 --help for the options and the list of extras.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	markdown2 "github.com/trentm/python-markdown2"
	"github.com/trentm/python-markdown2/internal/pyre"
	"github.com/trentm/python-markdown2/internal/pyunicode"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes the command and returns its exit status: 0 on success, 2 for a usage error and
// 1 when reading, converting or writing fails.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	out := bufio.NewWriter(stdout)
	status := execute(args, stdin, out, stderr)
	if err := out.Flush(); err != nil && status == 0 {
		fmt.Fprintf(stderr, "markdown2: %v\n", err)
		status = 1
	}
	return status
}

func execute(args []string, stdin io.Reader, out *bufio.Writer, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		var early *earlyExit
		if errors.As(err, &early) {
			out.WriteString(early.stdout)
			return 0
		}
		fmt.Fprintf(stderr, "usage: markdown2 [PATHS...]\nmarkdown2: error: %s\n", err)
		return 2
	}
	debug := func(msg string) {}
	if opts.verbose {
		debug = func(msg string) { fmt.Fprintf(stderr, "DEBUG:markdown:%s\n", msg) }
	}
	markdown2.SetDebugLogger(debug)

	if opts.selfTest {
		return selfTest(stderr)
	}

	var extras map[string]any
	if len(opts.extras) > 0 {
		extras = map[string]any{}
		splitter := pyre.MustCompile(`[,;: ]+`, 0)
		for _, s := range opts.extras {
			for _, e := range splitter.Split(s, 0) {
				if name, arg, found := strings.Cut(e, "="); found {
					if n, ok := pyunicode.ParseInt(arg); ok {
						extras[name] = n
					} else {
						extras[name] = arg
					}
				} else {
					extras[e] = nil
				}
			}
		}
	}

	var linkPatterns []markdown2.LinkPattern
	if opts.hasLinkPattern {
		linkPatterns, err = readLinkPatterns(opts.linkPatterns)
		if err != nil {
			return fail(stderr, err)
		}
	}

	paths := opts.paths
	if len(paths) == 0 {
		paths = []string{"-"}
	}
	stdinRead := false
	for _, path := range paths {
		var text string
		if path == "-" {
			data := []byte{}
			if !stdinRead {
				data, err = io.ReadAll(stdin)
				stdinRead = true
				if err != nil {
					return fail(stderr, err)
				}
			}
			// Standard input is decoded with Python's surrogateescape handler: bytes that are not
			// UTF-8 pass through to the output unchanged.
			text = strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
		} else {
			var data []byte
			data, err = os.ReadFile(path)
			if err == nil {
				text, err = markdown2.Decode(data, opts.encoding)
			}
		}
		if err != nil {
			return fail(stderr, err)
		}
		if opts.compare {
			if err := compareWithPerl(text, out, stderr); err != nil {
				return fail(stderr, err)
			}
		}
		mopts := markdown2.Options{
			HTML4Tags:    opts.html4tags,
			Extras:       extras,
			LinkPatterns: linkPatterns,
			UseFileVars:  opts.useFileVars,
			CLI:          true,
		}
		if opts.hasSafeMode {
			mopts.SafeMode = opts.safeMode
		}
		res, err := markdown2.Markdown(text, mopts)
		if err != nil {
			return fail(stderr, err)
		}
		if opts.hasOutput {
			if err := os.WriteFile(opts.output, []byte(res.HTML), 0o666); err != nil {
				return fail(stderr, err)
			}
		} else {
			out.WriteString(res.HTML)
		}
		if _, ok := extras["toc"]; ok {
			if res.TOCHTML == nil {
				return fail(stderr, errors.New("AttributeError: 'NoneType' object has no attribute 'encode'"))
			}
			debug("toc_html: " + pyunicode.BytesRepr([]byte(*res.TOCHTML)))
		}
		if opts.compare {
			fmt.Fprintf(out, "==== match? %s ====\n", "False")
		}
	}
	return 0
}

// fail reports an error that stops the command (exit status 1).
func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "markdown2: %v\n", err)
	return 1
}

// readLinkPatterns reads a link patterns file: one "PATTERN HREF" per line; blank lines and
// lines starting with # are skipped.
func readLinkPatterns(path string) ([]markdown2.LinkPattern, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, err := markdown2.Decode(data, "utf-8")
	if err != nil {
		return nil, err
	}
	patterns := []markdown2.LinkPattern{}
	for i, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		if pyunicode.TrimSpace(line) == "" || strings.HasPrefix(pyunicode.TrimLeftSpace(line), "#") {
			continue
		}
		fields := pyunicode.Fields(pyunicode.TrimRightSpace(line))
		if len(fields) < 2 {
			return nil, &markdown2.MarkdownError{Message: fmt.Sprintf("%s:%d: invalid link pattern line: %s",
				path, i+1, pyunicode.Repr(line))}
		}
		trimmed := pyunicode.TrimRightSpace(line)
		href := fields[len(fields)-1]
		pat := pyunicode.TrimRightSpace(strings.TrimSuffix(trimmed, href))
		if _, err := markdown2.New(markdown2.Options{LinkPatterns: []markdown2.LinkPattern{{Pattern: pat, Href: href}}}); err != nil {
			return nil, err
		}
		patterns = append(patterns, markdown2.LinkPattern{Pattern: pat, Href: href})
	}
	return patterns, nil
}

// compareWithPerl runs Markdown.pl from the repository's test directory on the text, for
// comparison with this implementation's output.
func compareWithPerl(text string, out *bufio.Writer, stderr io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	script := filepath.Join(filepath.Dir(filepath.Dir(exe)), "test", "Markdown.pl")
	fmt.Fprintln(out, "==== Markdown.pl ====")
	cmd := exec.Command("perl", script)
	cmd.Stdin = strings.NewReader(text)
	perlHTML, err := cmd.Output()
	if err != nil {
		// Like the original, report the comparison run's failure and go on with the conversion.
		fmt.Fprintf(stderr, "markdown2: running Markdown.pl: %v\n", err)
	}
	out.Write(perlHTML)
	fmt.Fprintln(out, "==== markdown2 ====")
	return nil
}

// selfTest checks a few conversions (the library's documented examples).
func selfTest(stderr io.Writer) int {
	checks := []struct{ in, want string }{
		{"*boo!*", "<p><em>boo!</em></p>\n"},
		{"**boom!**", "<p><strong>boom!</strong></p>\n"},
		{"\tfoo", "<pre><code>foo\n</code></pre>\n"},
	}
	for _, c := range checks {
		res, err := markdown2.Markdown(c.in, markdown2.Options{})
		if err != nil || res.HTML != c.want {
			fmt.Fprintf(stderr, "self-test failed for %q\n", c.in)
			return 1
		}
	}
	return 0
}
