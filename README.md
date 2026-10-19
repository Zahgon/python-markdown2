Markdown is a light text markup format and a processor to convert that to HTML.
The originator describes it as follows:

> Markdown is a text-to-HTML conversion tool for web writers.
> Markdown allows you to write using an easy-to-read,
> easy-to-write plain text format, then convert it to
> structurally valid XHTML (or HTML).
>
> -- <http://daringfireball.net/projects/markdown/>

This (markdown2) is a fast and complete Go implementation of Markdown,
ported from the Python markdown2 library. It was written to closely match the
behaviour of the original Perl-implemented Markdown.pl. Markdown2 also comes
with a number of extensions (called "extras") for things like syntax coloring,
tables, header-ids. See the "Extra Syntax" section below. The Go module needs
Go 1.23 or later and has no dependencies outside the standard library.

There is another [Python
markdown.py](https://python-markdown.github.io/). However, at
least at the time this project was started, markdown2.py was faster (see the
[Performance
Notes](https://github.com/trentm/python-markdown2/wiki/Performance-Notes)) and,
to my knowledge, more correct (see [Testing
Notes](https://github.com/trentm/python-markdown2/wiki/Testing-Notes)).
That was a while ago though, so you shouldn't discount Python-markdown from
your consideration.

Follow <a href="https://twitter.com/intent/user?screen_name=trentmick" target="_blank">@trentmick</a>
for updates to python-markdown2.

# Install

As a library:

    go get github.com/trentm/python-markdown2

The `markdown2` command line tool:

    go install github.com/trentm/python-markdown2/cmd/markdown2@latest

or, from a checkout, `make build` (or `go build ./cmd/markdown2`).


# Quick Usage

As a package:
```go
import markdown2 "github.com/trentm/python-markdown2"

res, err := markdown2.Markdown("*boo!*", markdown2.Options{})
// res.HTML == "<p><em>boo!</em></p>\n"

c, err := markdown2.New(markdown2.Options{})
res, err = c.Convert("*boo!*")
res, err = c.Convert("**boom!**")
// res.HTML == "<p><strong>boom!</strong></p>\n"
```
`markdown2.MarkdownPath(path, encoding, opts)` converts a file.

As a command line tool:
```shell
$ markdown2 foo.md > foo.html
$ markdown2 < foo.md > foo.html
```

Use `--use-file-vars` on the command line, or `UseFileVars: true` in the
`Options`, to enable extras declared in Emacs-style file variables such as
`<!-- -*- markdown-extras: header-ids -*- -->` at the top of a file. For
example, `<!-- -*- markdown-extras: toc -*- -->` enables a table of contents
and heading IDs for that document. This option is disabled by default and is
not an extra.

```shell
$ markdown2 --use-file-vars foo.md > foo.html
```

See the [project wiki](https://github.com/trentm/python-markdown2/wiki), the
package documentation (`go doc github.com/trentm/python-markdown2`) and/or
`markdown2 --help` for more details.


# Extra Syntax (aka extensions)

Many Markdown processors include support for additional optional syntax
(often called "extensions") and markdown2 is no exception. With markdown2 these
are called "extras".  Using the "footnotes" extra as an example, here is how
you use an extra ... as a command line tool:
```shell
$ markdown2 --extras footnotes foo.md > foo.html
```
as a package:
```go
res, err := markdown2.Markdown("*boo!*", markdown2.Options{Extras: map[string]any{"footnotes": nil}})
// res.HTML == "<p><em>boo!</em></p>\n"
```
An extra's argument (for example the options of `wiki-links`) is the value in
the `Extras` map: `nil`, a bool, a number, a string, a `[]any` or a
`map[string]any`.
The `footnotes` extra keeps the same number when a footnote is referenced more
than once.

The `tables` extra supports code spans delimited by single or multiple backticks,
including an operator cell such as ```` ``|`` ````. A pipe immediately after the
closing delimiter separates the next cell.

There are a number of currently implemented extras for tables, footnotes,
syntax coloring of `<pre>`-blocks, auto-linking patterns, table of contents,
Smarty Pants (for fancy quotes, dashes, etc.) and more. See the [Extras
wiki page](https://github.com/trentm/python-markdown2/wiki/Extras) for full
details.


# Project

The python-markdown2 project lives at
<https://github.com/trentm/python-markdown2/>.  (Note: On Mar 6, 2011 this
project was moved from [Google Code](http://code.google.com/p/python-markdown2)
to here on Github.)

The change log: <https://github.com/trentm/python-markdown2/blob/master/CHANGES.md>

To report a bug: <https://github.com/trentm/python-markdown2/issues>

# Contributing

We welcome pull requests from the community. Please take a look at the [TODO](https://github.com/trentm/python-markdown2/blob/master/TODO.txt) for opportunities to help this project. For those wishing to submit a pull request to `python-markdown2` please ensure it fulfills the following requirements:

* It must be gofmt-formatted and pass `go vet ./...`.
* It must include relevant test coverage.
* Bug fixes must include a regression test that exercises the bug.
* The entire test suite must pass.
* The README and/or docs are updated accordingly.


# Test Suite

This markdown implementation passes a fairly extensive test suite. To run it:
```shell
make test        # or: go test ./...
```
The crux of the test suite is a number of "cases" directories -- each with a
set of matching .text (input) and .html (expected output) files. These are:

    tm-cases/                   Tests authored for python-markdown2 (tm=="Trent Mick")
    markdowntest-cases/         Tests from the 3rd-party MarkdownTest package
    php-markdown-cases/         Tests from the 3rd-party MDTest package
    php-markdown-extra-cases/   Tests also from MDTest package

`go test ./...` converts every case (skipping those tagged `knownfailure`, or
needing the Python-only pygments, wavedrom, latex2mathml or emoji packages) and
compares the result with its `.html` file (and its `.toc_html` and `.metadata`
files when present). A case's `.opts` file holds the conversion options as a
Python-style literal. `make testredos` runs the checks that once-pathological
inputs convert quickly.

See the [Testing Notes wiki
page](https://github.com/trentm/python-markdown2/wiki/Testing-Notes) for full
details.
