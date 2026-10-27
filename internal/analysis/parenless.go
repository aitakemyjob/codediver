package analysis

import (
	"regexp"
	"strings"
)

// Calls without parentheses: in Ruby, `@store.save order`, `save!` and a
// bare `validate` are method calls. A receiver call (`recv.name`) is a call
// whatever follows it, so it goes through the same strategies as
// `recv.name(`. A bare name is only a call when it isn't a local variable
// and names a method of the caller's own class. Languages opt in with a
// parenlessHints entry.

// parenlessHints are one language's patterns for calls written without
// parentheses.
type parenlessHints struct {
	// recvCall matches `recv.name` (group 1 the receiver, group 2 the name).
	recvCall *regexp.Regexp
	// bare matches a bare lowercase name (group 1).
	bare *regexp.Regexp
	// locals match the names a method binds: assignments and block
	// parameters (group 1, which may list several names).
	locals []*regexp.Regexp
}

// parenlessCallHints holds the languages where calls need no parentheses.
var parenlessCallHints = map[string]*parenlessHints{
	"ruby": {
		recvCall: regexp.MustCompile(`(@{0,2}[A-Za-z_]\w*|self)\s*&?\.\s*([a-z_]\w*[?!]?)`),
		bare:     regexp.MustCompile(`(?:^|[^\w@$:.?!])([a-z_]\w*[?!]?)`),
		locals: []*regexp.Regexp{
			regexp.MustCompile(`^\s*([a-z_]\w*(?:\s*,\s*[a-z_]\w*)*)\s*(?:\|\||\+|-|\*|/)?=[^=~>]`),
			regexp.MustCompile(`\|([^|]*)\|`),
		},
	},
}

// parenlessCall is one call written without parentheses.
type parenlessCall struct {
	recv, name string
}

// parenlessCalls finds the calls on one masked line of fn written without
// parentheses: receiver calls, and bare names that aren't fn's locals.
// Calls followed by `(` are left to the `name(` matcher, and `x.name =`
// (an attribute write) isn't a call to name.
func parenlessCalls(h *parenlessHints, fn *Symbol, gf *genericFile, text string) []parenlessCall {
	var out []parenlessCall
	callAt := func(end int) bool {
		rest := strings.TrimLeft(text[end:], " \t")
		// `name(` is matched elsewhere; `name:` is a hash key and `Name::`
		// a constant path.
		if strings.HasPrefix(rest, "(") || strings.HasPrefix(rest, ":") {
			return false
		}
		// `x.name = v` writes an attribute.
		return !strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "==") || strings.HasPrefix(rest, "=~") || strings.HasPrefix(rest, "=>")
	}
	locals := methodLocals(h, fn, gf)
	covered := make([]bool, len(text)+1)
	for _, m := range h.recvCall.FindAllStringSubmatchIndex(text, -1) {
		for i := m[0]; i < m[1]; i++ {
			covered[i] = true
		}
		recv := text[m[2]:m[3]]
		// A receiver that is itself a method's result (`x.to_s.empty?`) has
		// no knowable class: don't guess which empty? it is.
		if chained := strings.TrimRight(text[:m[0]], " \t"); strings.HasSuffix(chained, ".") {
			continue
		}
		// A bare receiver that isn't a variable is itself a call:
		// `count.zero?` calls count.
		if isLowerIdent(recv) && recv != "self" && !locals[recv] && !keywords[recv] && !rubyKeywords[recv] {
			out = append(out, parenlessCall{name: recv})
		}
		if !callAt(m[1]) {
			continue
		}
		out = append(out, parenlessCall{recv: strings.TrimLeft(recv, "@"), name: text[m[4]:m[5]]})
	}
	for _, m := range h.bare.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[2]:m[3]]
		if covered[m[2]] || keywords[name] || rubyKeywords[name] || locals[name] || !callAt(m[3]) {
			continue
		}
		out = append(out, parenlessCall{name: name})
	}
	return out
}

// methodLocals returns the names fn binds: its parameters, assigned
// variables and block parameters.
func methodLocals(h *parenlessHints, fn *Symbol, gf *genericFile) map[string]bool {
	locals := map[string]bool{}
	for _, p := range fn.Params {
		locals[strings.TrimLeft(strings.TrimRight(paramName(p.Name), ":"), "*&")] = true
	}
	for ln := fn.Line; ln <= fn.EndLine && ln <= len(gf.lines); ln++ {
		for _, re := range h.locals {
			for _, m := range re.FindAllStringSubmatch(gf.lines[ln-1], -1) {
				for _, n := range strings.Split(m[1], ",") {
					if n = strings.TrimLeft(strings.TrimSpace(n), "*&"); n != "" {
						locals[n] = true
					}
				}
			}
		}
	}
	return locals
}

// isLowerIdent reports whether s is a plain lowercase identifier (not an
// instance variable or a constant).
func isLowerIdent(s string) bool {
	return s != "" && (s[0] == '_' || s[0] >= 'a' && s[0] <= 'z')
}

// rubyKeywords are Ruby words that look like bare method names.
var rubyKeywords = map[string]bool{}

// init fills rubyKeywords.
func init() {
	for _, k := range strings.Fields(`begin end then unless until elsif ensure rescue retry redo next nil true false
		defined module alias undef yield private protected public attr_reader attr_writer attr_accessor
		puts raise loop proc lambda block_given include extend require require_relative`) {
		rubyKeywords[k] = true
	}
}
