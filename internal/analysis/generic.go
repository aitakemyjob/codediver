package analysis

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// The heuristic analyzer covers non-Go languages with per-language regular
// expressions run over source whose comments and strings have been blanked
// out. It finds definitions, their extents, imports and name-matched calls.
// Everything it produces carries Heuristic=true.

type defPattern struct {
	re      *regexp.Regexp
	kind    SymbolKind
	name    int  // capture group of the name
	params  int  // capture group of the parameter list, 0 if none
	needsIn bool // only matches inside a class/impl body (methods)
	// in narrows needsIn to one container kind (KindClass, kindObject).
	in SymbolKind
}

// kindObject is an object literal assigned to a name or default-exported
// (`const api = {`, `export default {`). It holds methods like a class but
// isn't a symbol itself: its methods are qualified by its name.
const kindObject SymbolKind = "object"

// definable are keywords that may still name a definition: constructors,
// and methods such as delete or new (Rust's fn new, a store's delete).
var definable = map[string]bool{"constructor": true, "new": true, "delete": true}

type langSpec struct {
	defs    []defPattern
	imports []*regexp.Regexp
	// fileImports match across lines on the whole source (multi-line
	// `import {…} from`).
	fileImports []*regexp.Regexp
	block       string // "brace" | "indent" | "end"
	lineCmt     []string
	blockCmt    [2]string
	strDelims   string
	// exprBody marks languages where `= expr` after a signature is the
	// body (Kotlin, Scala: `fun f(x: Int) = x + 1`).
	exprBody bool
	// interp marks `${…}` in backtick strings as code (template literals).
	interp     bool
	extensions []string // used when resolving extension-less imports
}

// rx compiles a regular expression, panicking on error (patterns are
// constants).
func rx(s string) *regexp.Regexp { return regexp.MustCompile(s) }

var specs = map[string]*langSpec{
	"typescript": jsSpec(), "javascript": jsSpec(),
	"python": {
		defs: []defPattern{
			{re: rx(`^\s*class\s+([A-Za-z_]\w*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports:    []*regexp.Regexp{rx(`^\s*from\s+(\.*[\w.]*)\s+import`), rx(`^\s*import\s+([\w.]+)`)},
		block:      "indent",
		lineCmt:    []string{"#"},
		strDelims:  `"'`,
		extensions: []string{".py", "/__init__.py"},
	},
	"rust": {
		defs: []defPattern{
			{re: rx(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:struct|enum|union)\s+([A-Za-z_]\w*)`), kind: KindStruct, name: 1},
			{re: rx(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:unsafe\s+)?trait\s+([A-Za-z_]\w*)`), kind: KindInterface, name: 1},
			{re: rx(`^\s*impl(?:<[^>]*>)?\s+(?:[\w:<>]+\s+for\s+)?([A-Za-z_]\w*)`), kind: "impl", name: 1},
			{re: rx(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:const\s+)?(?:async\s+)?(?:unsafe\s+)?(?:extern\s+"[^"]*"\s+)?fn\s+([A-Za-z_]\w*)\s*(?:<[^>]*>)?\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports:    []*regexp.Regexp{rx(`^\s*(?:pub\s+)?mod\s+(\w+)\s*;`), rx(`^\s*use\s+(crate::[\w:]+)`)},
		block:      "brace",
		lineCmt:    []string{"//"},
		blockCmt:   [2]string{"/*", "*/"},
		strDelims:  `"`,
		extensions: []string{".rs", "/mod.rs"},
	},
	"java":   jvmSpec(`^\s*(?:@\w+\s+)*(?:(?:public|private|protected|static|final|abstract|synchronized|native|default)\s+)*(?:<[^>]+>\s+)?[\w<>\[\],.?]+\s+([a-zA-Z_]\w*)\s*\(([^)]*)\)?\s*(?:throws\s+[\w., ]+)?\s*(?:\{.*)?$`, ".java"),
	"csharp": jvmSpec(`^\s*(?:\[[^\]]*\]\s*)*(?:(?:public|private|protected|internal|static|virtual|override|abstract|async|sealed|extern|unsafe|new|partial)\s+)*[\w<>\[\],.?]+\s+([A-Za-z_]\w*)\s*(?:<[^>]*>)?\s*\(([^)]*)\)?\s*(?:where\s+[^{=]*)?\s*(?:\{.*|=>.*)?$`, ".cs"),
	"kotlin": {
		defs: []defPattern{
			{re: rx(`^\s*(?:(?:public|private|internal|protected|open|abstract|sealed|data|enum|inner|annotation)\s+)*(?:class|object)\s+([A-Za-z_]\w*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*(?:(?:public|private|internal|protected|sealed|fun)\s+)*interface\s+([A-Za-z_]\w*)`), kind: KindInterface, name: 1},
			{re: rx(`^\s*(?:(?:public|private|internal|protected|open|override|suspend|inline|operator|infix|tailrec|abstract)\s+)*fun\s+(?:<[^>]*>\s*)?(?:[\w.<>]+\.)?([A-Za-z_]\w*)\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports:  []*regexp.Regexp{rx(`^\s*import\s+([\w.]+)`)},
		exprBody: true, block: "brace", lineCmt: []string{"//"}, blockCmt: [2]string{"/*", "*/"}, strDelims: `"'`,
		extensions: []string{".kt"},
	},
	"swift": {
		defs: []defPattern{
			{re: rx(`^\s*(?:(?:public|private|fileprivate|internal|open|final)\s+)*(?:class|struct|enum|actor|extension)\s+([A-Za-z_]\w*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*(?:(?:public|private|fileprivate|internal|open)\s+)*protocol\s+([A-Za-z_]\w*)`), kind: KindInterface, name: 1},
			{re: rx(`^\s*(?:(?:public|private|fileprivate|internal|open|override|static|class|final|mutating|@\w+)\s+)*func\s+([A-Za-z_]\w*)\s*(?:<[^>]*>)?\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		block: "brace", lineCmt: []string{"//"}, blockCmt: [2]string{"/*", "*/"}, strDelims: `"`,
	},
	"scala": {
		defs: []defPattern{
			{re: rx(`^\s*(?:(?:case|abstract|final|sealed|private|protected|implicit)\s+)*(?:class|object|trait)\s+([A-Za-z_]\w*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*(?:(?:override|private|protected|final|implicit)\s+)*def\s+([A-Za-z_]\w*)\s*(?:\[[^\]]*\])?\s*\(?([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports:  []*regexp.Regexp{rx(`^\s*import\s+([\w.]+)`)},
		exprBody: true, block: "brace", lineCmt: []string{"//"}, blockCmt: [2]string{"/*", "*/"}, strDelims: `"`,
		extensions: []string{".scala"},
	},
	"dart": jvmSpec(`^\s*(?:(?:static|final|const|external|Future<[^>]*>|async)\s+)*[\w<>\[\],.?]+\s+([a-zA-Z_]\w*)\s*\(([^)]*)\)?\s*(?:async\s*)?(?:\{.*|=>.*)?$`, ".dart"),
	"c":    cSpec(),
	"cpp":  cSpec(),
	"ruby": {
		defs: []defPattern{
			{re: rx(`^\s*(?:class|module)\s+([A-Z]\w*(?:::\w+)*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*def\s+(?:self\.)?([A-Za-z_]\w*[?!=]?)\s*\(?([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports: []*regexp.Regexp{rx(`^\s*require_relative\s+['"]([^'"]+)['"]`)},
		block:   "end", lineCmt: []string{"#"}, strDelims: `"'`,
		extensions: []string{".rb"},
	},
	"php": {
		defs: []defPattern{
			{re: rx(`^\s*(?:(?:abstract|final)\s+)?(?:class|trait|enum)\s+([A-Za-z_]\w*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*interface\s+([A-Za-z_]\w*)`), kind: KindInterface, name: 1},
			{re: rx(`^\s*(?:(?:public|private|protected|static|abstract|final)\s+)*function\s+&?([A-Za-z_]\w*)\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports: []*regexp.Regexp{rx(`^\s*(?:require|include)(?:_once)?\s*\(?\s*['"]([^'"]+)['"]`)},
		block:   "brace", lineCmt: []string{"//", "#"}, blockCmt: [2]string{"/*", "*/"}, strDelims: `"'`,
	},
	"lua": {
		defs: []defPattern{
			{re: rx(`^\s*(?:local\s+)?function\s+([\w.:]+)\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
			{re: rx(`^\s*(?:local\s+)?([\w.]+)\s*=\s*function\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
		},
		imports: []*regexp.Regexp{rx(`require\s*\(?\s*['"]([^'"]+)['"]`)},
		block:   "end", lineCmt: []string{"--"}, strDelims: `"'`,
		extensions: []string{".lua"},
	},
}

// jsSpec describes TypeScript and JavaScript: functions, arrow functions,
// classes, interfaces, class methods and ES/CommonJS imports.
func jsSpec() *langSpec {
	return &langSpec{
		defs: []defPattern{
			{re: rx(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*(?:export\s+)?(?:declare\s+)?interface\s+([A-Za-z_$][\w$]*)`), kind: KindInterface, name: 1},
			// Anonymous default exports are named "default".
			{re: rx(`^\s*export\s+(default)\s+(?:async\s+)?function\s*\*?\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
			{re: rx(`^\s*export\s+(default)\s+(?:async\s+)?\(([^)]*)\)\s*(?::[^=]+)?=>`), kind: KindFunc, name: 1, params: 2},
			{re: rx(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)\s*(?:<[^>]*>)?\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
			{re: rx(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s+)?(?:function\s*\*?\s*)?\(([^)]*)\)?[^=]*=>`), kind: KindFunc, name: 1, params: 2},
			{re: rx(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s+)?function\b\s*\*?\s*\(([^)]*)`), kind: KindFunc, name: 1, params: 2},
			// Object literals holding methods: Workers' `export default {
			// fetch() {…} }`, `const api = { list() {…} }`.
			{re: rx(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*\{\s*$`), kind: kindObject, name: 1},
			{re: rx(`^\s*export\s+(default)\s*\{\s*$`), kind: kindObject, name: 1},
			// Class fields holding arrow functions: `onClick = (e) => {…}`.
			{re: rx(`^\s*(?:(?:public|private|protected|static|readonly|override)\s+)*([A-Za-z_$#][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s+)?(?:\(([^)]*)\)|[A-Za-z_$][\w$]*)\s*(?::[^=]+?)?=>`), kind: KindMethod, name: 1, params: 2, needsIn: true, in: KindClass},
			// Object-literal properties holding functions: `get: () => …`.
			{re: rx(`^\s*([A-Za-z_$][\w$]*)\s*:\s*(?:async\s+)?(?:function\s*\*?\s*)?(?:\(([^)]*)\)|[A-Za-z_$][\w$]*)\s*(?::[^=]+?)?(?:=>|\{)`), kind: KindMethod, name: 1, params: 2, needsIn: true, in: kindObject},
			// Methods; the body may continue on the same line.
			{re: rx(`^\s*(?:(?:public|private|protected|static|async|readonly|override|abstract|get|set)\s+)*\*?\s*([A-Za-z_$#][\w$]*)\s*(?:<[^>]*>)?\s*\(([^)]*)\)\s*(?::\s*[^{=;]+)?\{`), kind: KindMethod, name: 1, params: 2, needsIn: true},
		},
		imports: []*regexp.Regexp{
			rx(`^\s*import\s+(?:[^'"]*\s+from\s+)?['"]([^'"]+)['"]`),
			rx(`^\s*export\s+[^'"]*\s+from\s+['"]([^'"]+)['"]`),
			rx(`require\(\s*['"]([^'"]+)['"]\s*\)`),
		},
		fileImports: []*regexp.Regexp{
			rx(`(?m)^\s*(?:import|export)\s+(?:type\s+)?(?:[\w$]+\s*,\s*)?\{[^}]*\}\s*from\s*['"]([^'"]+)['"]`),
		},
		block:      "brace",
		lineCmt:    []string{"//"},
		blockCmt:   [2]string{"/*", "*/"},
		strDelims:  "\"'`",
		interp:     true,
		extensions: []string{".ts", ".tsx", ".js", ".jsx", ".mjs", "/index.ts", "/index.tsx", "/index.js"},
	}
}

// jvmSpec describes Java-like languages; method is the language's method
// declaration pattern and ext its source extension.
func jvmSpec(method, ext string) *langSpec {
	return &langSpec{
		defs: []defPattern{
			{re: rx(`^\s*(?:@\w+(?:\([^)]*\))?\s+)*(?:(?:public|private|protected|internal|static|final|abstract|sealed|partial|data)\s+)*(?:class|record|enum|struct)\s+([A-Za-z_]\w*)`), kind: KindClass, name: 1},
			{re: rx(`^\s*(?:(?:public|private|protected|internal)\s+)*(?:@)?interface\s+([A-Za-z_]\w*)`), kind: KindInterface, name: 1},
			{re: rx(method), kind: KindMethod, name: 1, params: 2},
		},
		imports:    []*regexp.Regexp{rx(`^\s*import\s+(?:static\s+)?([\w.]+)\s*;`), rx(`^\s*using\s+([\w.]+)\s*;`)},
		block:      "brace",
		lineCmt:    []string{"//"},
		blockCmt:   [2]string{"/*", "*/"},
		strDelims:  `"'`,
		extensions: []string{ext},
	}
}

// cSpec describes C and C++: top-level function definitions, classes,
// structs and quoted #includes.
func cSpec() *langSpec {
	return &langSpec{
		defs: []defPattern{
			{re: rx(`^\s*(?:template\s*<[^>]*>\s*)?(?:class|struct)\s+(?:\w+\s+)?([A-Za-z_]\w*)\s*(?:final\s*)?(?::[^;{]*)?\{?\s*$`), kind: KindClass, name: 1},
			{re: rx(`^(?:[A-Za-z_][\w:<>,*&\s]*?[\s*&])?((?:[A-Za-z_]\w*::)*~?[A-Za-z_]\w*)\s*\(([^;]*?)\)?\s*(?:const)?\s*(?:noexcept)?\s*(?:override)?\s*\{?\s*$`), kind: KindFunc, name: 1, params: 2},
		},
		imports:    []*regexp.Regexp{rx(`^\s*#\s*include\s+"([^"]+)"`)},
		block:      "brace",
		lineCmt:    []string{"//"},
		blockCmt:   [2]string{"/*", "*/"},
		strDelims:  `"'`,
		extensions: []string{".h", ".hpp", ".c", ".cc", ".cpp"},
	}
}

var keywords = map[string]bool{}

// init fills the keyword set excluded from call matching.
func init() {
	for _, k := range strings.Fields(`if else for while do switch case return function catch try finally throw new delete typeof
	instanceof void yield await async super this self import export from class def elif except with lambda pass raise
	sizeof static_assert defined match fn loop unsafe impl where print println printf require include not and or in is
	constructor`) {
		keywords[k] = true
	}
}

// goMask masks Go source, which go/ast parses rather than a langSpec.
var goMask = &langSpec{block: "brace", lineCmt: []string{"//"}, blockCmt: [2]string{"/*", "*/"}, strDelims: "\"'`"}

// MaskCode blanks the comments and string contents of src in lang, keeping
// line structure, so text-based scans only see code. Unknown languages are
// returned unchanged.
func MaskCode(src, lang string) string {
	sp := specs[lang]
	if lang == "go" {
		sp = goMask
	}
	if sp == nil {
		return src
	}
	return maskSource(src, sp)
}

// maskSource blanks comments and string contents (keeping newlines and
// quotes) so regexes never match inside them.
func maskSource(src string, sp *langSpec) string {
	b := []byte(src)
	n := len(b)
	for i := 0; i < n; {
		matched := false
		for _, lc := range sp.lineCmt {
			if strings.HasPrefix(src[i:], lc) {
				if lc == "#" && sp.block == "brace" && i+1 < n && b[i+1] == '[' {
					continue // PHP/Rust attributes are not comments
				}
				for i < n && b[i] != '\n' {
					b[i] = ' '
					i++
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if sp.blockCmt[0] != "" && strings.HasPrefix(src[i:], sp.blockCmt[0]) {
			end := strings.Index(src[i+len(sp.blockCmt[0]):], sp.blockCmt[1])
			stop := n
			if end >= 0 {
				stop = i + len(sp.blockCmt[0]) + end + len(sp.blockCmt[1])
			}
			for ; i < stop; i++ {
				if b[i] != '\n' {
					b[i] = ' '
				}
			}
			continue
		}
		c := b[i]
		if strings.IndexByte(sp.strDelims, c) >= 0 {
			// In Python f-strings, {…} fields are code; {{ and }} are text.
			fstr := sp.block == "indent" && fStringPrefix(src, i)
			field := func(j int) (int, bool) {
				if !fstr || (b[j] != '{' && b[j] != '}') {
					return j, false
				}
				if j+1 < n && b[j+1] == b[j] {
					b[j], b[j+1] = ' ', ' '
					return j + 2, true
				}
				if b[j] == '{' {
					return fieldEnd(b, j) + 1, true
				}
				return j, false
			}
			// Python triple-quoted strings.
			if sp.block == "indent" && strings.HasPrefix(src[i:], strings.Repeat(string(c), 3)) {
				q := strings.Repeat(string(c), 3)
				end := strings.Index(src[i+3:], q)
				stop := n
				if end >= 0 {
					stop = i + 3 + end + 3
				}
				for j := i + 3; j < stop-3 && j < n; {
					if next, ok := field(j); ok {
						j = next
						continue
					}
					if b[j] != '\n' {
						b[j] = ' '
					}
					j++
				}
				i = stop
				continue
			}
			j := i + 1
			for j < n && b[j] != c {
				// Template literal interpolations (`${total(o)}`) are code.
				if c == '`' && sp.interp && b[j] == '$' && j+1 < n && b[j+1] == '{' {
					j = fieldEnd(b, j+1) + 1
					continue
				}
				if next, ok := field(j); ok {
					j = next
					continue
				}
				if b[j] == '\\' {
					b[j] = ' '
					j++
					if j < n && b[j] != '\n' {
						b[j] = ' '
					}
					j++
					continue
				}
				if b[j] == '\n' && c != '`' {
					break
				}
				if b[j] != '\n' {
					b[j] = ' '
				}
				j++
			}
			i = j + 1
			continue
		}
		i++
	}
	return string(b)
}

// fStringPrefix reports whether the string opening at i is a Python
// f-string: its prefix (up to two of r, b, u, f) includes f.
func fStringPrefix(src string, i int) bool {
	f := false
	k := i - 1
	for ; k >= 0 && k >= i-2 && strings.IndexByte("rRbBuUfF", src[k]) >= 0; k-- {
		f = f || src[k] == 'f' || src[k] == 'F'
	}
	return f && (k < 0 || !isIdentByte(src[k]))
}

// isIdentByte reports whether c can be part of an identifier.
func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// fieldEnd returns the index of the brace closing the one at open
// (nested braces count), or the last index when it never closes.
func fieldEnd(b []byte, open int) int {
	depth := 0
	for j := open; j < len(b); j++ {
		switch b[j] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return j
			}
		}
	}
	return len(b) - 1
}

// indentOf returns the indentation width of line (tabs count as 4), or -1
// for blank lines.
func indentOf(line string) int {
	n := 0
	for _, r := range line {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return -1 // blank
}

// blockEnd returns the 1-based last line of the definition starting at idx.
func blockEnd(lines []string, idx int, sp *langSpec) int {
	switch sp.block {
	case "indent":
		base := indentOf(lines[idx])
		last := idx
		for j := idx + 1; j < len(lines); j++ {
			ind := indentOf(lines[j])
			if ind == -1 {
				continue
			}
			if ind <= base {
				break
			}
			last = j
		}
		return last + 1
	case "end":
		base := indentOf(lines[idx])
		for j := idx + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if indentOf(lines[j]) == base && (t == "end" || strings.HasPrefix(t, "end ") || strings.HasPrefix(t, "end)") || strings.HasPrefix(t, "end,")) {
				return j + 1
			}
		}
		return idx + 1
	default:
		return braceEnd(lines, idx, sp.exprBody)
	}
}

// braceEnd finds where a brace-language definition starting at line idx
// ends. Braces inside parentheses belong to the parameters (destructured
// props, default objects), and a brace right after `:`, `|`, `&`, `,` or
// `<` opens an object type (`): { a: string } {`), not the body. An arrow
// function whose body is an expression (`=> (<div>…</div>)`, `=> x + 1`)
// ends where that expression closes. Lines are masked, so strings and
// comments don't count.
func braceEnd(lines []string, idx int, exprBody bool) int {
	parens, braces, brackets, types := 0, 0, 0, 0
	opened, arrow, expr := false, false, false
	prev := byte(0) // last non-space character
	for j := idx; j < len(lines) && j < idx+4000; j++ {
		line := lines[j]
		for k := 0; k < len(line); k++ {
			c := line[k]
			last := prev
			if c != ' ' && c != '\t' {
				prev = c
			}
			if expr || arrow {
				// After `=>`, the first token decides: a brace is a body,
				// anything else an expression.
				if arrow && c != ' ' && c != '\t' {
					arrow = false
					expr = c != '{'
				}
			}
			switch c {
			case '(':
				parens++
			case ')':
				parens--
			case '[':
				brackets++
			case ']':
				brackets--
			case '{':
				if !opened && !expr && parens == 0 && brackets == 0 && (types > 0 || strings.IndexByte(":|&,<", last) >= 0) {
					types++ // an object type before the body
					continue
				}
				if parens == 0 && brackets == 0 && !expr && !opened {
					opened = true
					braces = 0
				}
				braces++
			case '}':
				if types > 0 && !opened {
					types--
					continue
				}
				braces--
			case '=':
				if k+1 < len(line) && line[k+1] == '>' && parens == 0 && !opened && !expr {
					arrow = true
					k++
				} else if exprBody && parens == 0 && brackets == 0 && types == 0 && !opened && !expr && !arrow && (k+1 >= len(line) || line[k+1] != '=') {
					// `fun f() = expr`: like an arrow, the next token decides.
					arrow = true
				}
			case ';':
				if !opened && !expr && parens == 0 && braces == 0 && types == 0 {
					return j + 1 // declaration only
				}
			}
			if opened && braces == 0 {
				return j + 1
			}
		}
		if expr && parens <= 0 && braces <= 0 && brackets <= 0 {
			// The expression closes on this line unless it visibly continues.
			t := strings.TrimRight(line, " \t")
			if !strings.HasSuffix(t, "=>") && !strings.HasSuffix(t, "(") && !strings.HasSuffix(t, "&&") && !strings.HasSuffix(t, "||") && !strings.HasSuffix(t, "?") && !strings.HasSuffix(t, ":") {
				return j + 1
			}
		}
		if !opened && !arrow && !expr && parens <= 0 && j > idx+3 {
			return idx + 1
		}
	}
	return idx + 1
}

// splitParams parses a parameter list into Params, handling generics,
// default values and both `name: type` and `type name` styles.
func splitParams(s string) []Param {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var parts []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(', '<', '[', '{':
			depth++
		case ')', '>', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	var out []Param
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" || p == "/" || p == "self" || p == "&self" || p == "&mut self" || p == "cls" {
			continue
		}
		if i := strings.Index(p, "="); i > 0 && !strings.Contains(p[:i], "=>") {
			p = strings.TrimSpace(p[:i]) // drop default value
		}
		name, typ := p, ""
		if i := strings.IndexAny(p, ":"); i > 0 && !strings.Contains(p[:i], "::") {
			name, typ = strings.TrimSpace(p[:i]), strings.TrimSpace(p[i+1:])
		} else if f := strings.Fields(p); len(f) >= 2 {
			name, typ = f[len(f)-1], strings.Join(f[:len(f)-1], " ")
		}
		if i := strings.Index(name, "="); i > 0 {
			name = strings.TrimSpace(name[:i])
		}
		if i := strings.Index(typ, "="); i > 0 {
			typ = strings.TrimSpace(typ[:i])
		}
		name = strings.TrimLeft(name, "*&")
		out = append(out, Param{Name: name, Type: typ})
	}
	return out
}

type genericFile struct {
	path    string
	lang    string
	lines   []string // masked
	raw     []string
	syms    []*Symbol
	imports []string
	// extends maps a class's ID to the name of the class it extends.
	extends map[string]string
}

var callRe = regexp.MustCompile(`(?:(this|self|[A-Za-z_$][\w$]*)\s*(?:\?\.|\.|::|->)\s*)?([A-Za-z_$][\w$]*)\s*\(`)

// extendsRe finds a class's parent in `class A extends B` (JS/TS, Java,
// Dart, Scala); qualified parents (ns.Base) keep their last name.
var extendsRe = regexp.MustCompile(`\bextends\s+(?:[A-Za-z_$][\w$]*\.)*([A-Za-z_$][\w$]*)`)

var (
	// jsxRe finds rendered components: <Foo …>, <Foo/>, <ns.Foo>. The tag
	// must not follow an identifier, which rules out generics (useState<T>).
	jsxRe = regexp.MustCompile(`(?:^|[^\w$.])<((?:[A-Za-z_$][\w$]*\.)*[A-Z][\w$]*)(?:[\s/>]|$)`)
	// componentRefRe finds components passed as values: component={Foo},
	// memo(Foo), forwardRef(Foo), { component: Foo }.
	componentRefRe = regexp.MustCompile(`=\{\s*([A-Z][\w$]*)\s*\}|\b(?:memo|forwardRef|observer)\(\s*([A-Z][\w$]*)\s*[,)]|\b(?:component|Component)\s*:\s*([A-Z][\w$]*)\b`)
)

// jsxFile reports whether a file may contain JSX.
func jsxFile(p string) bool {
	switch path.Ext(p) {
	case ".tsx", ".jsx", ".js", ".mjs":
		return true
	}
	return false
}

// analyzeGeneric indexes non-Go files lexically: definitions, directory
// level import edges and name-matched call edges (all marked heuristic).
func analyzeGeneric(root string, files []string, langOf func(string) string, ix *Index, merge map[[3]string]int) {
	var gfiles []*genericFile
	byName := map[string][]*Symbol{}
	fileSet := map[string]bool{}
	for _, f := range files {
		fileSet[f] = true
	}
	for _, f := range files {
		lang := langOf(f)
		sp := specs[lang]
		if sp == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || len(data) > 1<<20 {
			continue
		}
		src := string(data)
		gf := &genericFile{path: f, lang: lang, raw: strings.Split(src, "\n"), lines: strings.Split(maskSource(src, sp), "\n")}
		parseDefs(gf, sp)
		for _, s := range gf.syms {
			ix.addSymbol(s)
			if s.Kind.IsCallable() || s.Kind == KindClass {
				byName[s.Name] = append(byName[s.Name], s)
			}
		}
		// Imports use the unmasked text since the target is a string literal.
		for _, line := range gf.raw {
			for _, re := range sp.imports {
				if m := re.FindStringSubmatch(line); m != nil {
					gf.imports = append(gf.imports, m[1])
				}
			}
		}
		for _, re := range sp.fileImports {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				gf.imports = appendUnique(gf.imports, m[1])
			}
		}
		gfiles = append(gfiles, gf)
	}

	// Directory-level "packages" plus resolved file imports.
	for _, gf := range gfiles {
		dir := path.Dir(gf.path)
		if dir == "." {
			dir = ""
		}
		id := "dir:" + dir
		pkg := ix.Packages[id]
		if pkg == nil {
			name := path.Base(dir)
			if dir == "" {
				name = "(root)"
			}
			pkg = &Package{ID: id, Name: name, Dir: dir, Language: gf.lang, Internal: true}
			ix.Packages[id] = pkg
		}
		pkg.Files = append(pkg.Files, gf.path)
		for _, im := range gf.imports {
			target := resolveImport(gf.path, im, specs[gf.lang], fileSet)
			if target == "" {
				pkg.Imports = appendUnique(pkg.Imports, im)
				continue
			}
			tdir := path.Dir(target)
			if tdir == "." {
				tdir = ""
			}
			pkg.Imports = appendUnique(pkg.Imports, "dir:"+tdir)
		}
	}
	seenPkgEdge := map[[2]string]bool{}
	for id, pkg := range ix.Packages {
		if !strings.HasPrefix(id, "dir:") {
			continue
		}
		for _, im := range pkg.Imports {
			if _, ok := ix.Packages[im]; ok && im != id && !seenPkgEdge[[2]string{id, im}] {
				seenPkgEdge[[2]string{id, im}] = true
				ix.PkgEdges = append(ix.PkgEdges, Edge{From: id, To: im, Kind: EdgeImport, Heuristic: true})
			}
		}
	}

	// Call edges: through imports where they resolve, else by name.
	finder := newFinderChain(byName, newModuleGraph(gfiles, fileSet))
	for _, gf := range gfiles {
		for _, s := range gf.syms {
			if !s.Kind.IsCallable() {
				continue
			}
			// Lines of functions nested in s belong to them, not to s.
			var nested [][2]int
			for _, c := range gf.syms {
				if c != s && c.Kind.IsCallable() && c.Line > s.Line && c.EndLine <= s.EndLine {
					nested = append(nested, [2]int{c.Line, c.EndLine})
				}
			}
		lines:
			for ln := s.Line; ln <= s.EndLine && ln-1 < len(gf.lines); ln++ {
				for _, r := range nested {
					if ln >= r[0] && ln <= r[1] {
						continue lines
					}
				}
				text := gf.lines[ln-1]
				if ln == s.Line {
					// Skip the definition's own name.
					if i := strings.Index(text, s.Name); i >= 0 {
						text = text[i+len(s.Name):]
					}
				}
				for _, m := range callRe.FindAllStringSubmatch(text, -1) {
					recv, name := m[1], m[2]
					// Keywords aren't calls, except as methods: Store::new().
					if keywords[name] && (recv == "" || !definable[name]) {
						continue
					}
					target := finder.find(callSite{from: s, file: gf, recv: recv, name: name})
					if target == nil || target.ID == s.ID && recv == "" {
						continue
					}
					to := target.ID
					if target.Kind == KindClass {
						// `new Foo()` / `Foo()`: link to the constructor if present, else the class.
						for _, cand := range byName["constructor"] {
							if cand.Parent == target.ID {
								to = cand.ID
							}
						}
						for _, cand := range byName["__init__"] {
							if cand.Parent == target.ID {
								to = cand.ID
							}
						}
						// Java, C#, Kotlin, Dart: constructors share the class's name.
						for _, cand := range byName[target.Name] {
							if cand.Kind == KindMethod && cand.Parent == target.ID {
								to = cand.ID
							}
						}
					}
					ix.addEdge(Edge{From: s.ID, To: to, Kind: EdgeCall, Lines: []int{ln}, Heuristic: true}, merge)
				}
				if jsxFile(gf.path) {
					// Rendering a component is calling it; passing one is a reference.
					link := func(name string, kind EdgeKind) {
						recv := ""
						if i := strings.LastIndexByte(name, '.'); i >= 0 {
							recv, name = name[:i], name[i+1:]
							if j := strings.LastIndexByte(recv, '.'); j >= 0 {
								recv = recv[j+1:]
							}
						}
						if t := finder.find(callSite{from: s, file: gf, recv: recv, name: name}); t != nil && t.ID != s.ID {
							ix.addEdge(Edge{From: s.ID, To: t.ID, Kind: kind, Lines: []int{ln}, Heuristic: true}, merge)
						}
					}
					for _, m := range jsxRe.FindAllStringSubmatch(text, -1) {
						link(m[1], EdgeCall)
					}
					for _, m := range componentRefRe.FindAllStringSubmatch(text, -1) {
						link(m[1]+m[2]+m[3], EdgeRef)
					}
				}
			}
		}
	}
}

// superTarget finds the method a super.name() call reaches: the method
// called name on the class parent (or nil when the parent is unknown).
func superTarget(parent string, cands []*Symbol) *Symbol {
	if parent == "" {
		return nil
	}
	for _, c := range cands {
		if c.Kind == KindMethod && c.Receiver == parent {
			return c
		}
	}
	return nil
}

// appendUnique appends s to list unless it is already present.
func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// parseDefs finds the definitions in one file, tracking enclosing classes
// so methods get qualified names and parents.
func parseDefs(gf *genericFile, sp *langSpec) {
	type scope struct {
		sym *Symbol
		end int
	}
	var scopes []scope
	for i, line := range gf.lines {
		ln := i + 1
		for len(scopes) > 0 && ln > scopes[len(scopes)-1].end {
			scopes = scopes[:len(scopes)-1]
		}
		var container *Symbol
		for j := len(scopes) - 1; j >= 0; j-- {
			k := scopes[j].sym.Kind
			if k == KindClass || k == KindInterface || k == KindStruct || k == "impl" || k == kindObject {
				container = scopes[j].sym
				break
			}
		}
		insideFunc := len(scopes) > 0 && scopes[len(scopes)-1].sym.Kind.IsCallable()
		for _, d := range sp.defs {
			if d.needsIn && (container == nil || insideFunc || d.in != "" && container.Kind != d.in) {
				continue
			}
			m := d.re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name := m[d.name]
			if keywords[name] && !definable[name] || name == "" {
				continue
			}
			// C-family method heuristic must not fire on calls / control flow.
			trimmed := strings.TrimSpace(line)
			if d.kind == KindMethod || (d.kind == KindFunc && (gf.lang == "c" || gf.lang == "cpp")) {
				if strings.HasSuffix(trimmed, ";") && !strings.Contains(trimmed, "=>") || strings.HasPrefix(trimmed, "return") || strings.HasPrefix(trimmed, "else") || insideFunc {
					continue
				}
				if (gf.lang == "c" || gf.lang == "cpp") && indentOf(gf.lines[i]) > 0 && container == nil {
					continue
				}
			}
			end := blockEnd(gf.lines, i, sp)
			kind := d.kind
			if kind == KindFunc || kind == KindMethod {
				kind = KindFunc
				if container != nil {
					kind = KindMethod
				}
			}
			s := &Symbol{
				Name: name, Qualified: name, Kind: kind, Language: gf.lang, File: gf.path,
				Line: ln, EndLine: end, Heuristic: true,
				Signature: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(gf.raw[i]), "{")),
			}
			if d.params > 0 && d.params < len(m) {
				s.Params = splitParams(m[d.params])
			}
			if container != nil && kind == KindMethod {
				cname := container.Name
				s.Parent = container.ID
				switch container.Kind {
				case "impl":
					// Attach Rust impl methods to the struct when we know it.
					s.Parent = "h:" + gf.path + "#" + cname
				case kindObject:
					s.Parent = "" // the object literal isn't a symbol
				}
				s.Receiver = cname
				s.Qualified = cname + "." + name
			}
			s.ID = fmt.Sprintf("h:%s#%s", gf.path, s.Qualified)
			if kind == "impl" || kind == kindObject {
				s.ID = fmt.Sprintf("h:%s#%s %s@%d", gf.path, kind, name, ln)
			}
			s.Exported = isExportedGuess(gf.lang, name, gf.raw[i])
			s.Complexity = heuristicComplexity(gf.lines[i:min(end, len(gf.lines))])
			if prev := docAbove(gf.raw, i, sp); prev != "" {
				s.Doc = prev
			}
			if kind != "impl" && kind != kindObject {
				gf.syms = append(gf.syms, s)
			}
			if container != nil && kind == KindMethod {
				container.Methods = append(container.Methods, s.ID)
			}
			if kind == KindClass {
				if m := extendsRe.FindStringSubmatch(line); m != nil {
					if gf.extends == nil {
						gf.extends = map[string]string{}
					}
					gf.extends[s.ID] = m[1]
				}
			}
			scopes = append(scopes, scope{sym: s, end: end})
			break
		}
	}
	// De-duplicate IDs (overloads) by suffixing the line.
	seen := map[string]bool{}
	for _, s := range gf.syms {
		if seen[s.ID] {
			s.ID = fmt.Sprintf("%s@%d", s.ID, s.Line)
		}
		seen[s.ID] = true
	}
}

// isExportedGuess approximates visibility from language conventions
// (export keyword, leading underscore, pub, private).
func isExportedGuess(lang, name, line string) bool {
	switch lang {
	case "typescript", "javascript":
		return strings.Contains(line, "export ")
	case "python":
		return !strings.HasPrefix(name, "_")
	case "rust":
		return strings.Contains(line, "pub ")
	case "java", "csharp", "kotlin", "swift", "php", "scala", "dart":
		return !strings.Contains(line, "private ")
	}
	return true
}

var branchRe = regexp.MustCompile(`\b(if|elif|else if|for|while|case|catch|except|when)\b|&&|\|\||\?\s`)

// heuristicComplexity estimates cyclomatic complexity by counting branch
// keywords and boolean operators.
func heuristicComplexity(lines []string) int {
	c := 1
	for _, l := range lines {
		c += len(branchRe.FindAllString(l, -1))
	}
	return c
}

// docAbove returns the doc comment for the definition on line i: the
// comment block above it, or a Python docstring below it.
func docAbove(raw []string, i int, sp *langSpec) string {
	if sp.block == "indent" {
		// Python docstring is the first statement in the body.
		if i+1 < len(raw) {
			t := strings.TrimSpace(raw[i+1])
			for _, q := range []string{`"""`, `'''`} {
				if strings.HasPrefix(t, q) {
					body := strings.TrimPrefix(t, q)
					if j := strings.Index(body, q); j >= 0 {
						return strings.TrimSpace(body[:j])
					}
					var sb []string
					sb = append(sb, body)
					for k := i + 2; k < len(raw) && k < i+30; k++ {
						if j := strings.Index(raw[k], q); j >= 0 {
							sb = append(sb, strings.TrimSpace(raw[k][:j]))
							break
						}
						sb = append(sb, strings.TrimSpace(raw[k]))
					}
					return strings.TrimSpace(strings.Join(sb, "\n"))
				}
			}
		}
		return ""
	}
	var docs []string
	for j := i - 1; j >= 0 && j >= i-40; j-- {
		t := strings.TrimSpace(raw[j])
		switch {
		case strings.HasPrefix(t, "///"), strings.HasPrefix(t, "//"), strings.HasPrefix(t, "#") && sp.block != "brace":
			docs = append([]string{strings.TrimSpace(strings.TrimLeft(t, "/#!"))}, docs...)
		case strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/**") || strings.HasPrefix(t, "/*") || strings.HasSuffix(t, "*/"):
			t = strings.TrimSpace(strings.TrimSuffix(strings.TrimLeft(t, "/*"), "*/"))
			if t != "" && !strings.HasPrefix(t, "@") {
				docs = append([]string{t}, docs...)
			}
		case strings.HasPrefix(t, "@"), strings.HasPrefix(t, "#["), strings.HasPrefix(t, "["):
			continue // decorators/attributes
		default:
			return strings.TrimSpace(strings.Join(docs, "\n"))
		}
	}
	return strings.TrimSpace(strings.Join(docs, "\n"))
}

// pickTarget chooses the most plausible definition for a called name.
func pickTarget(from *Symbol, recv string, cands []*Symbol, file string) *Symbol {
	if len(cands) == 0 {
		return nil
	}
	// Methods of built-in types (int.Parse, Math.max, String.valueOf)
	// are never the repository's code.
	if builtinTypes[recv] {
		return nil
	}
	if recv == "this" || recv == "self" {
		for _, c := range cands {
			if c.Parent != "" && c.Parent == from.Parent {
				return c
			}
		}
	}
	var sameFile, sameDir []*Symbol
	dir := path.Dir(file)
	for _, c := range cands {
		if c.File == file {
			sameFile = append(sameFile, c)
		} else if path.Dir(c.File) == dir {
			sameDir = append(sameDir, c)
		}
	}
	// A receiver-qualified call (obj.method) should land on a method. When
	// the method name is also a built-in container or string method
	// (data.get, items.push), the receiver has to look like the class
	// (store.get → MemoryStore.get); an unknown receiver is more likely a
	// dict, list or string.
	filter := func(list []*Symbol) []*Symbol {
		if recv == "" {
			return list
		}
		var out []*Symbol
		for _, c := range list {
			if c.Kind != KindMethod && c.Kind != KindClass && c.Kind != KindFunc {
				continue
			}
			if c.Kind == KindMethod && builtinMethods[c.Name] && recv != "this" && recv != "self" && !receiverMatches(recv, c.Receiver) {
				continue
			}
			out = append(out, c)
		}
		return out
	}
	for _, group := range [][]*Symbol{filter(sameFile), filter(sameDir), filter(cands)} {
		if len(group) == 1 {
			return group[0]
		}
		if len(group) > 1 && len(group) <= 3 {
			return group[0]
		}
	}
	return nil
}

// builtinTypes are standard type and namespace names whose static
// methods share names with repository code (Parse, max, valueOf).
var builtinTypes = map[string]bool{}

// init fills builtinTypes.
func init() {
	for _, t := range strings.Fields(`int long short byte sbyte uint ulong ushort float double decimal bool char string object
		Integer Long Short Byte Float Double Boolean Character String Object Math Arrays Collections Objects Optional
		Convert Guid DateTime TimeSpan Enum Console Environment Path File Directory Task Enumerable
		JSON Number Array Promise Date Reflect Symbol BigInt i32 i64 u32 u64 usize f64 str Vec HashMap`) {
		builtinTypes[t] = true
	}
}

// builtinMethods are method names common on built-in dicts, lists, maps,
// sets, strings and promises in Python and JavaScript.
var builtinMethods = map[string]bool{}

// init fills builtinMethods.
func init() {
	for _, m := range strings.Fields(`get set has add delete clear pop push shift unshift append extend insert remove
		update keys values items entries copy sort reverse index count find filter map reduce forEach some every
		includes join split strip replace startswith endswith lower upper format then catch finally slice splice
		concat setdefault discard toString trim`) {
		builtinMethods[m] = true
	}
}

// receiverMatches reports whether a call's receiver variable plausibly
// holds an instance of class: one name contains the other, ignoring case
// and underscores (store ~ MemoryStore, order_repo ~ OrderRepo).
func receiverMatches(recv, class string) bool {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }
	r, c := norm(recv), norm(class)
	return r != "" && c != "" && (strings.Contains(c, r) || strings.Contains(r, c))
}

// resolveImport maps an import spec to a repo file, when possible.
func resolveImport(from, spec string, sp *langSpec, files map[string]bool) string {
	dir := path.Dir(from)
	try := func(base string) string {
		base = path.Clean(base)
		if files[base] {
			return base
		}
		for _, ext := range sp.extensions {
			if files[base+ext] {
				return base + ext
			}
		}
		return ""
	}
	switch {
	case strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../"):
		return try(path.Join(dir, spec))
	case strings.HasPrefix(spec, "crate::"):
		parts := strings.Split(strings.TrimPrefix(spec, "crate::"), "::")
		for n := len(parts); n > 0; n-- {
			if t := try(path.Join("src", path.Join(parts[:n]...))); t != "" {
				return t
			}
		}
	case strings.HasPrefix(spec, "."): // python relative
		dots := len(spec) - len(strings.TrimLeft(spec, "."))
		base := dir
		for i := 1; i < dots; i++ {
			base = path.Dir(base)
		}
		return try(path.Join(base, strings.ReplaceAll(strings.TrimLeft(spec, "."), ".", "/")))
	default:
		// Python/Java style dotted names or C includes, resolved against the
		// importing file's dir and then any file with a matching suffix.
		rel := spec
		if !strings.Contains(spec, "/") && strings.Contains(spec, ".") && !strings.HasSuffix(spec, ".h") && !strings.HasSuffix(spec, ".hpp") {
			rel = strings.ReplaceAll(spec, ".", "/")
		}
		if t := try(path.Join(dir, rel)); t != "" {
			return t
		}
		if t := try(rel); t != "" {
			return t
		}
		if sp.block == "indent" || strings.Contains(rel, "/") {
			for f := range files {
				for _, ext := range append([]string{""}, sp.extensions...) {
					if strings.HasSuffix(f, "/"+rel+ext) {
						return f
					}
				}
			}
		}
		// `mod foo;` in Rust resolves next to the declaring file.
		if t := try(path.Join(dir, spec)); t != "" {
			return t
		}
	}
	return ""
}
