// Go port of python-markdown2. Copyright (c) 2012 Trent Mick. Copyright (c) 2007-2008 ActiveState Corp. (python-markdown2)
// License: MIT (http://www.opensource.org/licenses/mit-license.php)

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/trentm/python-markdown2/internal/pyunicode"
)

// The command line follows the rules of the argparse parser the tool has always used: long
// options may be abbreviated to any unambiguous prefix, short options may be bundled (-vx toc)
// or carry their value attached (-xtoc, -x=toc), "--" ends the options, an argument that looks
// like a negative number is a path, and the paths are taken from one run of arguments only.

type actionKind int

const (
	actHelp actionKind = iota
	actVersion
	actStoreConst // -v
	actStore      // takes one value
	actStoreTrue
	actAppend
)

type action struct {
	name    string // dest
	strings []string
	kind    actionKind
}

// displayName is how errors name an option: its option strings joined by "/".
func (a *action) displayName() string { return strings.Join(a.strings, "/") }

var actions = []*action{
	{"help", []string{"-h", "--help"}, actHelp},
	{"version", []string{"--version"}, actVersion},
	{"log_level", []string{"-v", "--verbose"}, actStoreConst},
	{"encoding", []string{"--encoding"}, actStore},
	{"html4tags", []string{"--html4tags"}, actStoreTrue},
	{"safe_mode", []string{"-s", "--safe"}, actStore},
	{"extras", []string{"-x", "--extras"}, actAppend},
	{"use_file_vars", []string{"--use-file-vars"}, actStoreTrue},
	{"link_patterns_file", []string{"--link-patterns-file"}, actStore},
	{"self_test", []string{"--self-test"}, actStoreTrue},
	{"compare", []string{"--compare"}, actStoreTrue},
	{"output", []string{"--output"}, actStore},
}

// optionStrings lists every option string in definition order.
var optionStrings = func() []string {
	var out []string
	for _, a := range actions {
		out = append(out, a.strings...)
	}
	return out
}()

func lookup(opt string) *action {
	for _, a := range actions {
		for _, s := range a.strings {
			if s == opt {
				return a
			}
		}
	}
	return nil
}

func takesValue(a *action) bool { return a.kind == actStore || a.kind == actAppend }

type options struct {
	verbose        bool
	encoding       string
	html4tags      bool
	safeMode       string
	hasSafeMode    bool
	extras         []string
	useFileVars    bool
	linkPatterns   string
	hasLinkPattern bool
	selfTest       bool
	compare        bool
	output         string
	hasOutput      bool
	paths          []string
}

// usageError is an argparse error: the usage line and the message, exit status 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// earlyExit is --help or --version, which print and stop as soon as they are parsed.
type earlyExit struct{ stdout string }

func (e *earlyExit) Error() string { return "exit" }

type optionTuple struct {
	act         *action
	option      string
	sep         string
	hasSep      bool
	explicit    string
	hasExplicit bool
}

var negativeNumber = regexp.MustCompile(`^-\d+$|^-\d*\.\d+$`)

// parseOptional classifies one argument: nil means a positional argument.
func parseOptional(arg string) []optionTuple {
	if arg == "" || arg[0] != '-' {
		return nil
	}
	if a := lookup(arg); a != nil {
		return []optionTuple{{act: a, option: arg}}
	}
	if len([]rune(arg)) == 1 {
		return nil
	}
	if opt, explicit, found := strings.Cut(arg, "="); found {
		if a := lookup(opt); a != nil {
			return []optionTuple{{act: a, option: opt, sep: "=", hasSep: true, explicit: explicit, hasExplicit: true}}
		}
	}
	if tuples := optionTuples(arg); len(tuples) > 0 {
		return tuples
	}
	if negativeNumber.MatchString(arg) {
		return nil
	}
	if strings.Contains(arg, " ") {
		return nil
	}
	return []optionTuple{{option: arg}}
}

func optionTuples(arg string) []optionTuple {
	var result []optionTuple
	rs := []rune(arg)
	if len(rs) > 1 && rs[0] == '-' && rs[1] == '-' {
		prefix, explicit, found := strings.Cut(arg, "=")
		for _, s := range optionStrings {
			if strings.HasPrefix(s, prefix) {
				result = append(result, optionTuple{act: lookup(s), option: s, sep: "=", hasSep: found, explicit: explicit, hasExplicit: found})
			}
		}
		return result
	}
	prefix, explicit, found := strings.Cut(arg, "=")
	short := string(rs[:2])
	shortExplicit := string(rs[2:])
	for _, s := range optionStrings {
		if s == short {
			result = append(result, optionTuple{act: lookup(s), option: s, hasSep: true, explicit: shortExplicit, hasExplicit: true})
		} else if strings.HasPrefix(s, prefix) {
			result = append(result, optionTuple{act: lookup(s), option: s, sep: "=", hasSep: found, explicit: explicit, hasExplicit: found})
		}
	}
	return result
}

// parseArgs parses the command line as argparse does.
func parseArgs(args []string) (*options, error) {
	opts := &options{encoding: "utf-8"}
	var pattern []byte
	tuples := map[int][]optionTuple{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			pattern = append(pattern, '-')
			for i++; i < len(args); i++ {
				pattern = append(pattern, 'A')
			}
			break
		}
		if t := parseOptional(args[i]); t != nil {
			tuples[i] = t
			pattern = append(pattern, 'O')
		} else {
			pattern = append(pattern, 'A')
		}
	}

	var extras []string
	positionalsDone := false

	take := func(a *action, values []string) error {
		switch a.kind {
		case actHelp:
			return &earlyExit{stdout: helpText}
		case actVersion:
			return &earlyExit{stdout: "markdown2 2.5.6\n"}
		case actStoreConst:
			opts.verbose = true
		case actStoreTrue:
			switch a.name {
			case "html4tags":
				opts.html4tags = true
			case "use_file_vars":
				opts.useFileVars = true
			case "self_test":
				opts.selfTest = true
			case "compare":
				opts.compare = true
			}
		case actStore:
			v := values[0]
			switch a.name {
			case "encoding":
				opts.encoding = v
			case "safe_mode":
				opts.safeMode, opts.hasSafeMode = v, true
			case "link_patterns_file":
				opts.linkPatterns, opts.hasLinkPattern = v, true
			case "output":
				opts.output, opts.hasOutput = v, true
			}
		case actAppend:
			opts.extras = append(opts.extras, values[0])
		}
		return nil
	}

	consumeOptional := func(start int) (int, error) {
		ts := tuples[start]
		if len(ts) > 1 {
			var names []string
			for _, t := range ts {
				names = append(names, t.option)
			}
			return 0, &usageError{fmt.Sprintf("ambiguous option: %s could match %s", args[start], strings.Join(names, ", "))}
		}
		t := ts[0]
		type taken struct {
			act    *action
			values []string
		}
		var todo []taken
		stop := start + 1
		for {
			if t.act == nil {
				extras = append(extras, args[start])
				return start + 1, nil
			}
			if t.hasExplicit {
				argCount := 0
				if takesValue(t.act) {
					argCount = 1
				}
				optRunes := []rune(t.option)
				if argCount == 0 && optRunes[1] != '-' && t.explicit != "" {
					if t.sep != "" || strings.HasPrefix(t.explicit, "-") {
						return 0, &usageError{fmt.Sprintf("argument %s: ignored explicit argument %s", t.act.displayName(), pyunicode.Repr(t.explicit))}
					}
					todo = append(todo, taken{t.act, nil})
					er := []rune(t.explicit)
					next := "-" + string(er[0])
					if a := lookup(next); a != nil {
						rest := string(er[1:])
						t = optionTuple{act: a, option: next}
						switch {
						case rest == "":
						case rest[0] == '=':
							t.sep, t.hasSep, t.explicit, t.hasExplicit = "=", true, rest[1:], true
						default:
							t.hasSep, t.explicit, t.hasExplicit = true, rest, true
						}
						continue
					}
					extras = append(extras, "-"+t.explicit)
					stop = start + 1
					break
				} else if argCount == 1 {
					todo = append(todo, taken{t.act, []string{t.explicit}})
					stop = start + 1
					break
				}
				return 0, &usageError{fmt.Sprintf("argument %s: ignored explicit argument %s", t.act.displayName(), pyunicode.Repr(t.explicit))}
			}
			if takesValue(t.act) {
				if start+1 >= len(pattern) || pattern[start+1] != 'A' {
					return 0, &usageError{fmt.Sprintf("argument %s: expected one argument", t.act.displayName())}
				}
				todo = append(todo, taken{t.act, []string{args[start+1]}})
				stop = start + 2
			} else {
				todo = append(todo, taken{t.act, nil})
				stop = start + 1
			}
			break
		}
		for _, x := range todo {
			if err := take(x.act, x.values); err != nil {
				return 0, err
			}
		}
		return stop, nil
	}

	// consumePositionals gives the paths positional (nargs '*') the run of arguments at start.
	consumePositionals := func(start int) int {
		if positionalsDone {
			return start
		}
		end := start
		for end < len(pattern) && (pattern[end] == 'A' || pattern[end] == '-') {
			end++
		}
		positionalsDone = true
		removed := false
		for i := start; i < end; i++ {
			if pattern[i] == '-' && !removed {
				removed = true
				continue
			}
			opts.paths = append(opts.paths, args[i])
		}
		return end
	}

	maxOption := -1
	for i := range tuples {
		if i > maxOption {
			maxOption = i
		}
	}
	start := 0
	for start <= maxOption {
		next := len(args)
		for i := range tuples {
			if i >= start && i < next {
				next = i
			}
		}
		if start != next {
			end := consumePositionals(start)
			if end > start {
				start = end
				continue
			}
		}
		if _, isOpt := tuples[start]; !isOpt {
			extras = append(extras, args[start:next]...)
			start = next
		}
		stop, err := consumeOptional(start)
		if err != nil {
			return nil, err
		}
		start = stop
	}
	stop := consumePositionals(start)
	extras = append(extras, args[stop:]...)
	if len(extras) > 0 {
		return nil, &usageError{"unrecognized arguments: " + strings.Join(extras, " ")}
	}
	return opts, nil
}
