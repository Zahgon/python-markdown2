// Go port of python-markdown2. Copyright (c) 2007-2008 ActiveState Software Inc. (python-markdown2 test suite)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fileVarsSource = "<!-- -*- markdown-extras: header-ids -*- -->\n# Heading\n"

// runCLI runs the command in-process and requires it to succeed.
func runCLI(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if status := run(args, strings.NewReader(stdin), &stdout, &stderr); status != 0 {
		t.Fatalf("markdown2 %s: exit status %d, stderr %q", strings.Join(args, " "), status, stderr.String())
	}
	return stdout.String()
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileVarsFromStdin(t *testing.T) {
	if html := runCLI(t, fileVarsSource, "--use-file-vars"); !strings.Contains(html, `<h1 id="heading">Heading</h1>`) {
		t.Errorf("markdown2 --use-file-vars < source = %q, want a header with id heading", html)
	}
}

func TestFileVarsBeforeAndAfterPath(t *testing.T) {
	path := writeFile(t, t.TempDir(), "input.md", fileVarsSource)
	for _, args := range [][]string{{"--use-file-vars", path}, {path, "--use-file-vars"}} {
		if html := runCLI(t, "", args...); !strings.Contains(html, `<h1 id="heading">Heading</h1>`) {
			t.Errorf("markdown2 %v = %q, want a header with id heading", args, html)
		}
	}
}

func TestFileVarsWithMultiplePaths(t *testing.T) {
	dir := t.TempDir()
	first := writeFile(t, dir, "first.md", fileVarsSource)
	second := writeFile(t, dir, "second.md", fileVarsSource)
	html := runCLI(t, "", "--use-file-vars", first, second)
	if n := strings.Count(html, `<h1 id="heading">Heading</h1>`); n != 2 {
		t.Errorf("markdown2 --use-file-vars first.md second.md has %d headers with id heading, want 2:\n%s", n, html)
	}
}

func TestFileVarsWithOutputOption(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output.html")
	if stdout := runCLI(t, fileVarsSource, "--use-file-vars", "--output", output); stdout != "" {
		t.Errorf("markdown2 --output wrote %q to stdout, want nothing", stdout)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `<h1 id="heading">Heading</h1>`) {
		t.Errorf("output file = %q, want a header with id heading", data)
	}
}

func TestFileVarsAreDisabledByDefault(t *testing.T) {
	html := runCLI(t, fileVarsSource)
	if !strings.Contains(html, "<h1>Heading</h1>") || strings.Contains(html, `id="heading"`) {
		t.Errorf("markdown2 < source = %q, want a header without id", html)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"--foo"}, "unrecognized arguments: --foo"},
		{[]string{"--ver"}, "ambiguous option: --ver could match --version, --verbose"},
		{[]string{"-s"}, "argument -s/--safe: expected one argument"},
		{[]string{"a", "--html4tags", "b"}, "unrecognized arguments: b"},
		{[]string{"--html4tags=1"}, "argument --html4tags: ignored explicit argument '1'"},
	}
	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		status := run(tc.args, strings.NewReader(""), &stdout, &stderr)
		want := "usage: markdown2 [PATHS...]\nmarkdown2: error: " + tc.msg + "\n"
		if status != 2 || stderr.String() != want {
			t.Errorf("markdown2 %v: status %d, stderr %q; want 2, %q", tc.args, status, stderr.String(), want)
		}
	}
}

func TestVersion(t *testing.T) {
	if got := runCLI(t, "", "--version"); got != "markdown2 2.5.6\n" {
		t.Errorf("markdown2 --version = %q, want %q", got, "markdown2 2.5.6\n")
	}
}
