package analysis

import "strings"

// String interpolation: the code inside `"#{f(x)}"`, `${…}`, `\(…)` or an
// f-string field is code, not string text, so masking keeps it. Each
// language lists the string forms that interpolate; adding a language is
// adding its rules here.

// interpRule is one way a language puts code inside a string literal.
type interpRule struct {
	// quotes are the delimiters of the strings it applies to.
	quotes string
	// prefix, when set, reports whether the string opening at src[i]
	// carries the prefix that makes it interpolate (f"…", $"…", s"…").
	prefix func(src string, i int) bool
	// open starts the embedded code; its last byte is the bracket that the
	// code runs to the match of: `{` or `(`.
	open string
	// doubled marks `{{` and `}}` as literal braces (f-strings, C#).
	doubled bool
}

// interpolations holds each language's interpolating string forms. Kotlin
// and Dart `$name` and Swift-style bare names can't contain calls, so only
// the bracketed forms are listed.
var interpolations = map[string][]interpRule{
	"typescript": {{quotes: "`", open: "${"}},
	"javascript": {{quotes: "`", open: "${"}},
	"python":     {{quotes: `"'`, prefix: fStringPrefix, open: "{", doubled: true}},
	"ruby":       {{quotes: `"`, open: "#{"}},
	"kotlin":     {{quotes: `"`, open: "${"}},
	"dart":       {{quotes: `"'`, open: "${"}},
	"csharp":     {{quotes: `"`, prefix: csInterpPrefix, open: "{", doubled: true}},
	"swift":      {{quotes: `"`, open: `\(`}},
	"scala":      {{quotes: `"`, prefix: scalaInterpPrefix, open: "${"}},
	"php":        {{quotes: `"`, open: "{$"}},
}

// init attaches each language's interpolation rules to its spec.
func init() {
	for lang, rules := range interpolations {
		specs[lang].interps = rules
	}
}

// interpsAt returns the rules that apply to the string opened by quote at
// src[i].
func (sp *langSpec) interpsAt(src string, i int, quote byte) []interpRule {
	var out []interpRule
	for _, r := range sp.interps {
		if strings.IndexByte(r.quotes, quote) >= 0 && (r.prefix == nil || r.prefix(src, i)) {
			out = append(out, r)
		}
	}
	return out
}

// interpField reports whether embedded code (or a doubled literal brace)
// starts at b[j] under rules, and returns the index just past it. Code is
// left as is; doubled braces are blanked.
func interpField(b []byte, j int, rules []interpRule) (int, bool) {
	for _, r := range rules {
		if r.doubled && j+1 < len(b) && (b[j] == '{' || b[j] == '}') && b[j+1] == b[j] {
			b[j], b[j+1] = ' ', ' '
			return j + 2, true
		}
		if j+len(r.open) <= len(b) && string(b[j:j+len(r.open)]) == r.open {
			return bracketEnd(b, j+len(r.open)-1) + 1, true
		}
	}
	return j, false
}

// fStringPrefix reports whether the string opening at i is a Python
// f-string: its prefix (up to two of r, b, u, f) includes f.
func fStringPrefix(src string, i int) bool {
	return stringPrefix(src, i, "rRbBuUfF", "fF")
}

// csInterpPrefix reports whether the C# string opening at i is
// interpolated: `$"…"`, `$@"…"` or `@$"…"`.
func csInterpPrefix(src string, i int) bool {
	return stringPrefix(src, i, "$@", "$")
}

// scalaInterpPrefix reports whether the Scala string opening at i has an
// interpolator: any identifier right before the quote (s"…", f"…", raw"…").
func scalaInterpPrefix(src string, i int) bool {
	return i > 0 && isIdentByte(src[i-1]) && src[i-1] != '$'
}

// stringPrefix reports whether the up to two characters before the quote
// at i are all from allowed, include one from required, and don't continue
// an identifier.
func stringPrefix(src string, i int, allowed, required string) bool {
	found := false
	k := i - 1
	for ; k >= 0 && k >= i-2 && strings.IndexByte(allowed, src[k]) >= 0; k-- {
		found = found || strings.IndexByte(required, src[k]) >= 0
	}
	return found && (k < 0 || !isIdentByte(src[k]))
}

// bracketEnd returns the index of the bracket closing the `{` or `(` at
// open (nested ones count), or the last index when it never closes.
func bracketEnd(b []byte, open int) int {
	o := b[open]
	c := byte('}')
	if o == '(' {
		c = ')'
	}
	depth := 0
	for j := open; j < len(b); j++ {
		switch b[j] {
		case o:
			depth++
		case c:
			if depth--; depth == 0 {
				return j
			}
		}
	}
	return len(b) - 1
}

// sameClassMethod returns the method among cands defined on from's own
// class, or nil. A bare name could be a local variable, so only a method
// of the caller's class counts.
func sameClassMethod(from *Symbol, cands []*Symbol) *Symbol {
	if from.Parent == "" {
		return nil
	}
	for _, c := range cands {
		if c.Kind == KindMethod && c.Parent == from.Parent && c.ID != from.ID {
			return c
		}
	}
	return nil
}
