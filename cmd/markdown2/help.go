// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package main

// helpText is the --help output: the usage line, the description of the extras and the options.
const helpText = "usage: markdown2 [PATHS...]\n" +
	"\n" +
	"A fast and complete Python implementation of Markdown, a\n" +
	"text-to-HTML conversion tool for web writers.\n" +
	"\n" +
	"Supported extra syntax options (see -x|--extras option below and\n" +
	"see <https://github.com/trentm/python-markdown2/wiki/Extras> for details):\n" +
	"\n" +
	"* admonitions: Enable parsing of RST admonitions.\n" +
	"* breaks: Control where hard breaks are inserted in the markdown.\n" +
	"  Options include:\n" +
	"  - on_newline: Replace single new line characters with <br> when True\n" +
	"  - on_backslash: Replace backslashes at the end of a line with <br>\n" +
	"* break-on-newline: Alias for the on_newline option in the breaks extra.\n" +
	"* code-friendly: Disable _ and __ for em and strong.\n" +
	"* cuddled-lists: Allow lists to be cuddled to the preceding paragraph.\n" +
	"* emojis: add emoji support using emoji codes\n" +
	"* fenced-code-blocks: Allows a code block to not have to be indented\n" +
	"  by fencing it with '```' on a line before and after. Based on\n" +
	"  <http://github.github.com/github-flavored-markdown/> with support for\n" +
	"  syntax highlighting.\n" +
	"* footnotes: Support footnotes as in use on daringfireball.net and\n" +
	"  implemented in other Markdown processors (tho not in Markdown.pl v1.0.1).\n" +
	"* header-ids: Adds \"id\" attributes to headers. The id value is a slug of\n" +
	"  the header text.\n" +
	"* highlightjs-lang: Allows specifying the language which used for syntax\n" +
	"  highlighting when using fenced-code-blocks and highlightjs.\n" +
	"* html-classes: Takes a dict mapping html tag names (lowercase) to a\n" +
	"  string to use for a \"class\" tag attribute. Currently only supports \"img\",\n" +
	"  \"table\", \"thead\", \"pre\", \"code\", \"ul\" and \"ol\" tags. Add an issue if you require\n" +
	"  this for other tags.\n" +
	"* link-patterns: Auto-link given regex patterns in text (e.g. bug number\n" +
	"  references, revision number references).\n" +
	"* link-shortrefs: allow shortcut reference links, not followed by `[]` or\n" +
	"  a link label.\n" +
	"* markdown-file-links: Replace links to `.md` files with `.html` links\n" +
	"* markdown-in-html: Allow the use of `markdown=\"1\"` or simply a `markdown` boolean\n" +
	"  attribute in a block HTML tag to have markdown processing be done on its\n" +
	"  contents. Similar to\n" +
	"  <http://michelf.com/projects/php-markdown/extra/#markdown-attr> but with\n" +
	"  some limitations.\n" +
	"* metadata: Extract metadata from a leading '---'-fenced block.\n" +
	"  See <https://github.com/trentm/python-markdown2/issues/77> for details.\n" +
	"* middle-word-em: Allows or disallows emphasis syntax in the middle of words,\n" +
	"  defaulting to allow. Disabling this means that `this_text_here` will not be\n" +
	"  converted to `this<em>text</em>here`.\n" +
	"* nofollow: Add `rel=\"nofollow\"` to add `<a>` tags with an href. See\n" +
	"  <http://en.wikipedia.org/wiki/Nofollow>.\n" +
	"* numbering: Support of generic counters.  Non standard extension to\n" +
	"  allow sequential numbering of figures, tables, equations, exhibits etc.\n" +
	"* pyshell: Treats unindented Python interactive shell sessions as <code>\n" +
	"  blocks.\n" +
	"* smarty-pants: Replaces ' and \" with curly quotation marks or curly\n" +
	"  apostrophes.  Replaces --, ---, ..., and . . . with en dashes, em dashes,\n" +
	"  and ellipses.\n" +
	"* spoiler: A special kind of blockquote commonly hidden behind a\n" +
	"  click on SO. Syntax per <http://meta.stackexchange.com/a/72878>.\n" +
	"* strike: text inside of double tilde is ~~strikethrough~~\n" +
	"* tag-friendly: Requires atx style headers to have a space between the # and\n" +
	"  the header text. Useful for applications that require twitter style tags to\n" +
	"  pass through the parser.\n" +
	"* tables: Tables using the same format as GFM\n" +
	"  <https://help.github.com/articles/github-flavored-markdown#tables> and\n" +
	"  PHP-Markdown Extra <https://michelf.ca/projects/php-markdown/extra/#table>.\n" +
	"* toc: The returned HTML string gets a new \"toc_html\" attribute which is\n" +
	"  a Table of Contents for the document. (experimental)\n" +
	"* wiki-links: Wiki-style `[[Page Name]]` links. The generated URL and CSS class\n" +
	"  can be configured via the \"base_url\", \"end_url\", \"html_class\" and \"build_url\"\n" +
	"  options.\n" +
	"* wiki-tables: Google Code Wiki-style tables. See\n" +
	"  <http://code.google.com/p/support/wiki/WikiSyntax#Tables>.\n" +
	"* wavedrom: Support for generating Wavedrom digital timing diagrams\n" +
	"* xml: Passes one-liner processing instructions and namespaced XML tags.\n" +
	"\n" +
	"positional arguments:\n" +
	"  paths                 optional list of files to convert.If none are given,\n" +
	"                        stdin will be used\n" +
	"\n" +
	"options:\n" +
	"  -h, --help            show this help message and exit\n" +
	"  --version             show program's version number and exit\n" +
	"  -v, --verbose         more verbose output\n" +
	"  --encoding ENCODING   specify encoding of text content\n" +
	"  --html4tags           use HTML 4 style for empty element tags\n" +
	"  -s MODE, --safe MODE  sanitize literal HTML: 'escape' escapes HTML meta\n" +
	"                        chars, 'replace' replaces with an [HTML_REMOVED] note\n" +
	"  -x EXTRAS, --extras EXTRAS\n" +
	"                        Turn on specific extra features (not part of the core\n" +
	"                        Markdown spec). See above.\n" +
	"  --use-file-vars       Look for and use Emacs-style 'markdown-extras' file\n" +
	"                        var to turn on extras. See\n" +
	"                        <https://github.com/trentm/python-\n" +
	"                        markdown2/wiki/Extras>\n" +
	"  --link-patterns-file LINK_PATTERNS_FILE\n" +
	"                        path to a link pattern file\n" +
	"  --self-test           run internal self-tests (some doctests)\n" +
	"  --compare             run against Markdown.pl as well (for testing)\n" +
	"  --output OUTPUT       output to a file instead of stdout\n"
