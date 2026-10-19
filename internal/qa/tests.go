package qa

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

// Test kinds.
const (
	KindTest      = "test"
	KindSubtest   = "subtest"
	KindCase      = "case"
	KindSuite     = "suite"
	KindBenchmark = "benchmark"
	KindFuzz      = "fuzz"
	KindExample   = "example"
	KindTodo      = "todo"
)

// TestCase is one test, subtest, table case or suite found in a test file.
type TestCase struct {
	ID string `json:"id"` // file:line
	// Name is the test's name as written (TestFoo, "closes on Escape").
	Name string `json:"name"`
	// Behavior is Name as a readable sentence.
	Behavior  string   `json:"behavior"`
	Kind      string   `json:"kind"`
	Framework string   `json:"framework"`
	File      string   `json:"file"`
	Line      int      `json:"line"`
	EndLine   int      `json:"endLine"`
	Parent    string   `json:"parent,omitempty"`
	Suite     []string `json:"suite,omitempty"` // enclosing describe/class names
	Skipped   bool     `json:"skipped,omitempty"`
	// Targets are the IDs of code symbols the test calls directly; filled
	// in by the caller, which owns the symbol index.
	Targets []string `json:"targets,omitempty"`
}

// IsTestFile reports whether a repo-relative path is a test file by the
// naming conventions of common languages and frameworks.
func IsTestFile(p string) bool {
	base := path.Base(p)
	lower := strings.ToLower(base)
	ext := path.Ext(lower)
	stem := strings.TrimSuffix(base, path.Ext(base))
	dirs := "/" + strings.ToLower(path.Dir(p)) + "/"
	switch ext {
	case ".go":
		return strings.HasSuffix(lower, "_test.go")
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".vue", ".svelte":
		return strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") || strings.Contains(dirs, "/__tests__/") || strings.Contains(dirs, "/__mocks__/")
	case ".py":
		return strings.HasPrefix(lower, "test_") || strings.HasSuffix(lower, "_test.py") || (strings.Contains(dirs, "/tests/") && lower != "__init__.py" && lower != "conftest.py")
	case ".java", ".kt", ".kts", ".scala", ".groovy":
		return strings.Contains(dirs, "/src/test/") || strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") || strings.HasSuffix(stem, "Spec") || strings.HasSuffix(stem, "IT")
	case ".cs", ".fs", ".vb":
		return strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") || strings.Contains(dirs, ".tests/") || strings.Contains(dirs, ".test/")
	case ".rb":
		return strings.HasSuffix(lower, "_spec.rb") || strings.HasSuffix(lower, "_test.rb") || strings.HasPrefix(lower, "test_")
	case ".php":
		return strings.HasSuffix(stem, "Test")
	case ".swift":
		return strings.HasSuffix(stem, "Tests") || strings.HasSuffix(stem, "Test")
	case ".rs":
		return strings.Contains(dirs, "/tests/")
	case ".c", ".cc", ".cpp", ".cxx":
		return strings.HasSuffix(stem, "_test") || strings.HasSuffix(stem, "_unittest") || strings.HasPrefix(lower, "test_")
	case ".dart":
		return strings.HasSuffix(lower, "_test.dart")
	case ".ex", ".exs":
		return strings.HasSuffix(lower, "_test.exs")
	}
	return false
}

// MayContainTests reports whether a non-test-named file should still be
// scanned (Rust keeps unit tests in #[cfg(test)] modules beside the code).
func MayContainTests(p, content string) bool {
	return strings.HasSuffix(p, ".rs") && strings.Contains(content, "#[test]")
}

// Discover finds the tests declared in a file.
func Discover(file, lang, content string) []TestCase {
	d := &discoverer{file: file, src: content, lines: strings.Split(content, "\n")}
	switch lang {
	case "go":
		d.golang()
	case "typescript", "javascript", "typescriptreact", "javascriptreact", "vue", "svelte":
		d.jsLike(file)
	case "dart":
		d.jsLike(file)
	case "python":
		d.python()
	case "ruby":
		d.ruby()
	case "rust":
		d.annotated(rustAttr, regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+(\w+)`), "rust")
	case "java", "scala", "groovy":
		d.annotated(javaAnno, regexp.MustCompile(`(\w+)\s*\([^)]*\)\s*(?:throws[\w\s,.]+)?\{?\s*$`), "junit")
	case "kotlin":
		d.annotated(javaAnno, regexp.MustCompile("fun\\s+(`[^`]+`|\\w+)\\s*\\("), "junit")
	case "csharp":
		d.annotated(csAttr, regexp.MustCompile(`(?:public|private|internal|protected)?\s*(?:async\s+)?(?:static\s+)?[\w<>\[\],.]+\s+(\w+)\s*\(`), "dotnet")
	case "swift":
		d.methods(regexp.MustCompile(`^\s*(?:@\w+\s+)*func\s+(test\w*)\s*\(`), "xctest")
	case "php":
		d.methods(regexp.MustCompile(`^\s*(?:public\s+)?function\s+(test\w*)\s*\(`), "phpunit")
	case "cpp", "c":
		d.methods(regexp.MustCompile(`^\s*TEST(?:_F|_P)?\s*\(\s*(\w+)\s*,\s*(\w+)\s*\)`), "gtest")
	}
	return d.out
}

type discoverer struct {
	file  string
	src   string
	lines []string
	out   []TestCase
}

// add records a test case and returns its ID.
func (d *discoverer) add(tc TestCase) string {
	tc.File = d.file
	tc.ID = fmt.Sprintf("%s:%d", d.file, tc.Line)
	if tc.Behavior == "" {
		tc.Behavior = Humanize(tc.Name)
	}
	if tc.EndLine < tc.Line {
		tc.EndLine = tc.Line
	}
	for _, x := range d.out {
		if x.ID == tc.ID {
			return x.ID
		}
	}
	d.out = append(d.out, tc)
	return tc.ID
}

// offsetLine converts a byte offset into a 1-based line number.
func (d *discoverer) offsetLine(off int) int { return strings.Count(d.src[:off], "\n") + 1 }

// lineOffset returns the byte offset of the start of a 1-based line.
func (d *discoverer) lineOffset(line int) int {
	off := 0
	for i := 0; i < line-1 && i < len(d.lines); i++ {
		off += len(d.lines[i]) + 1
	}
	return min(off, len(d.src))
}

// blockEnd returns the line of the brace closing the first '{' at or
// after line (or the same line if none is found).
func (d *discoverer) blockEnd(line int, style syntax) int {
	off := d.lineOffset(line)
	open := strings.IndexByte(d.src[off:], '{')
	if open < 0 {
		return line
	}
	if end := matchClose(d.src, off+open, style); end > 0 {
		return d.offsetLine(end)
	}
	return line
}

var (
	goTestFunc = regexp.MustCompile(`^func\s+(Test|Benchmark|Fuzz|Example)(\w*)\s*\(`)
	goRun      = regexp.MustCompile(`\b\w+\.Run\(\s*("(?:[^"\\]|\\.)*"|` + "`[^`]*`" + `)`)
	goCaseName = regexp.MustCompile(`(?i)^\s*\{?\s*(?:name|desc|description|title|scenario)\s*:\s*("(?:[^"\\]|\\.)*")`)
	// Only an unconditional, top-level t.Skip marks a test skipped;
	// "skip if git is missing" guards are nested inside an if.
	goSkip = regexp.MustCompile(`(?m)^\t\w+\.Skip(?:f|Now)?\(|^func .*\{[ \t]*t\.Skip(?:f|Now)?\(`)
)

// golang finds Test/Benchmark/Fuzz/Example functions, t.Run subtests and
// named table-driven cases.
func (d *discoverer) golang() {
	for i, l := range d.lines {
		m := goTestFunc.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		// Helpers like TestMain or testHelper are not tests.
		if m[1] == "Test" && (m[2] == "Main" || (m[2] != "" && !unicode.IsUpper(rune(m[2][0])) && m[2][0] != '_')) {
			continue
		}
		line := i + 1
		end := d.blockEnd(line, cSyntax)
		kind := map[string]string{"Test": KindTest, "Benchmark": KindBenchmark, "Fuzz": KindFuzz, "Example": KindExample}[m[1]]
		body := strings.Join(d.lines[i:min(end, len(d.lines))], "\n")
		name := m[1] + m[2]
		id := d.add(TestCase{Name: name, Behavior: Humanize(name), Kind: kind, Framework: "go test", Line: line, EndLine: end, Skipped: goSkip.MatchString(body)})
		for j := line; j < end && j <= len(d.lines); j++ {
			ln := d.lines[j-1]
			if sm := goRun.FindStringSubmatchIndex(ln); sm != nil {
				lit := unquote(ln[sm[2]:sm[3]])
				off := d.lineOffset(j) + sm[0]
				sub := j
				if p := strings.IndexByte(d.src[off:], '('); p >= 0 {
					if e := matchClose(d.src, off+p, cSyntax); e > 0 {
						sub = d.offsetLine(e)
					}
				}
				d.add(TestCase{Name: lit, Kind: KindSubtest, Framework: "go test", Line: j, EndLine: sub, Parent: id, Suite: []string{name}})
			} else if cm := goCaseName.FindStringSubmatch(ln); cm != nil {
				d.add(TestCase{Name: unquote(cm[1]), Kind: KindCase, Framework: "go test", Line: j, EndLine: j, Parent: id, Suite: []string{name}})
			}
		}
	}
}

var (
	jsCall   = regexp.MustCompile(`(?m)(?:^|[^\w.$])(x?describe|x?context|suite|x?it|test|specify|f?it|fdescribe|group)((?:\.(?:only|skip|todo|concurrent|sequential|each|failing))*)\s*(\(|` + "`" + `)`)
	jsString = regexp.MustCompile(`^\s*(?:\[[^\]]*\]\s*\)\s*\(\s*)?(` + "'(?:[^'\\\\]|\\\\.)*'|\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`" + `)`)
)

// jsLike finds describe/it/test blocks (Jest, Vitest, Mocha, Jasmine,
// node:test, Playwright, Dart's test package) with their nesting.
func (d *discoverer) jsLike(file string) {
	framework := "jest/vitest"
	switch {
	case strings.Contains(d.src, "from 'vitest'") || strings.Contains(d.src, `from "vitest"`):
		framework = "vitest"
	case strings.Contains(d.src, "@playwright/test"):
		framework = "playwright"
	case strings.Contains(d.src, "node:test"):
		framework = "node:test"
	case strings.HasSuffix(file, ".dart"):
		framework = "dart test"
	}
	type block struct {
		id         string
		name       string
		start, end int
	}
	var suites []block
	for _, m := range jsCall.FindAllStringSubmatchIndex(d.src, -1) {
		fn := d.src[m[2]:m[3]]
		mods := d.src[m[4]:m[5]]
		open := m[6]
		args := d.src[open+1:]
		if d.src[open] != '(' {
			continue
		}
		// .each([...])('name', ...) — skip past the table to the name.
		if strings.Contains(mods, "each") {
			if e := matchClose(d.src, open, jsSyntax); e > 0 && e+1 < len(d.src) && d.src[e+1] == '(' {
				open = e + 1
				args = d.src[open+1:]
			}
		}
		sm := jsString.FindStringSubmatch(args)
		if sm == nil {
			continue
		}
		name := unquote(sm[1])
		end := matchClose(d.src, open, jsSyntax)
		if end < 0 {
			end = open
		}
		line, endLine := d.offsetLine(m[2]), d.offsetLine(end)
		var chain []string
		parent := ""
		for _, s := range suites {
			if s.start <= m[2] && m[2] <= s.end {
				chain = append(chain, s.name)
				parent = s.id
			}
		}
		skipped := strings.HasPrefix(fn, "x") || strings.Contains(mods, "skip")
		kind := KindTest
		if strings.Contains(fn, "describe") || fn == "context" || fn == "xcontext" || fn == "suite" || fn == "group" {
			kind = KindSuite
		} else if strings.Contains(mods, "todo") {
			kind = KindTodo
		}
		id := d.add(TestCase{Name: name, Behavior: sentence(name), Kind: kind, Framework: framework, Line: line, EndLine: endLine, Parent: parent, Suite: chain, Skipped: skipped})
		if kind == KindSuite {
			suites = append(suites, block{id: id, name: name, start: m[2], end: end})
		}
	}
}

var (
	pyDef   = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+(test\w*)\s*\(`)
	pyClass = regexp.MustCompile(`^(\s*)class\s+(Test\w*|\w+Test(?:s|Case)?)\b`)
	pyDoc   = regexp.MustCompile(`^\s*(?:[rRuU]?)("""|''')\s*(.*?)\s*(?:"""|''')?\s*$`)
	pySkip  = regexp.MustCompile(`@(?:pytest\.mark\.skip|unittest\.skip)`)
)

// python finds pytest/unittest test functions, methods and classes, using
// indentation for scope and the docstring's first line as the behavior.
func (d *discoverer) python() {
	type cls struct {
		id, name string
		indent   int
	}
	var stack []cls
	names := func() []string {
		var out []string
		for _, c := range stack {
			out = append(out, c.name)
		}
		return out
	}
	indentOf := func(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }
	scopeEnd := func(i, indent int) int {
		end := i + 1
		for j := i + 1; j < len(d.lines); j++ {
			t := strings.TrimSpace(d.lines[j])
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if indentOf(d.lines[j]) <= indent {
				break
			}
			end = j + 1
		}
		return end
	}
	for i, l := range d.lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := indentOf(l)
		for len(stack) > 0 && ind <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		if m := pyClass.FindStringSubmatch(l); m != nil {
			name := m[2]
			id := d.add(TestCase{Name: name, Behavior: Humanize(name), Kind: KindSuite, Framework: "pytest", Line: i + 1, EndLine: scopeEnd(i, ind), Suite: names()})
			stack = append(stack, cls{id, name, ind})
			continue
		}
		m := pyDef.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		name := m[2]
		behavior := Humanize(name)
		// Signatures can span lines; the docstring follows the line ending in ':'.
		for j := i; j < min(i+8, len(d.lines)); j++ {
			if strings.HasSuffix(strings.TrimSpace(strings.SplitN(d.lines[j], "#", 2)[0]), ":") {
				if j+1 < len(d.lines) {
					if dm := pyDoc.FindStringSubmatch(d.lines[j+1]); dm != nil && dm[2] != "" {
						behavior = sentence(strings.TrimSuffix(dm[2], "."))
					}
				}
				break
			}
		}
		parent := ""
		if len(stack) > 0 {
			parent = stack[len(stack)-1].id
		}
		skipped := i > 0 && pySkip.MatchString(d.lines[i-1])
		d.add(TestCase{Name: name, Behavior: behavior, Kind: KindTest, Framework: "pytest", Line: i + 1, EndLine: scopeEnd(i, ind), Parent: parent, Suite: names(), Skipped: skipped})
	}
}

var (
	rbBlock = regexp.MustCompile(`^(\s*)(describe|context|it|specify|scenario|feature|test)\s*\(?\s*([\w:]+|'(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*")`)
	rbDef   = regexp.MustCompile(`^(\s*)def\s+(test_\w+)`)
)

// ruby finds RSpec describe/context/it blocks and minitest test_ methods.
func (d *discoverer) ruby() {
	type blk struct {
		id, name string
		indent   int
	}
	var stack []blk
	indentOf := func(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }
	endOf := func(i, indent int) int {
		for j := i + 1; j < len(d.lines); j++ {
			if strings.TrimSpace(d.lines[j]) == "end" && indentOf(d.lines[j]) == indent {
				return j + 1
			}
		}
		return i + 1
	}
	for i, l := range d.lines {
		ind := indentOf(l)
		for len(stack) > 0 && ind <= stack[len(stack)-1].indent && strings.TrimSpace(l) != "" {
			stack = stack[:len(stack)-1]
		}
		var chain []string
		parent := ""
		for _, s := range stack {
			chain = append(chain, s.name)
			parent = s.id
		}
		if m := rbBlock.FindStringSubmatch(l); m != nil {
			name := unquote(m[3])
			kind := KindTest
			if m[2] == "describe" || m[2] == "context" || m[2] == "feature" {
				kind = KindSuite
			}
			id := d.add(TestCase{Name: name, Behavior: sentence(name), Kind: kind, Framework: "rspec", Line: i + 1, EndLine: endOf(i, ind), Parent: parent, Suite: chain})
			if kind == KindSuite {
				stack = append(stack, blk{id, name, ind})
			}
		} else if m := rbDef.FindStringSubmatch(l); m != nil {
			d.add(TestCase{Name: m[2], Kind: KindTest, Framework: "minitest", Line: i + 1, EndLine: endOf(i, ind), Parent: parent, Suite: chain})
		}
	}
}

var (
	javaAnno    = regexp.MustCompile(`^\s*@(Test|ParameterizedTest|RepeatedTest|TestFactory|TestTemplate|Property)\b`)
	rustAttr    = regexp.MustCompile(`^\s*#\[(?:[\w:]+::)?(test|rstest|quickcheck)\b`)
	csAttr      = regexp.MustCompile(`^\s*\[(?:[\w.]+\.)?(Fact|Theory|Test|TestMethod|TestCase|DataTestMethod)\b`)
	displayName = regexp.MustCompile(`(?:@DisplayName\(\s*|DisplayName\s*=\s*)("(?:[^"\\]|\\.)*")`)
	skipAnno    = regexp.MustCompile(`@Disabled|@Ignore|#\[ignore\]|Skip\s*=`)
	classDecl   = regexp.MustCompile(`\b(?:class|object|mod|impl)\s+(\w+)`)
)

// annotated finds tests marked by an annotation or attribute on the line(s)
// before the declaration (JUnit, Kotest, xUnit/NUnit/MSTest, Rust).
func (d *discoverer) annotated(marker, decl *regexp.Regexp, framework string) {
	style := cSyntax
	if framework == "rust" {
		style = rustSyntax
	}
	type cls struct {
		name       string
		start, end int
	}
	var classes []cls
	for i, l := range d.lines {
		if m := classDecl.FindStringSubmatch(l); m != nil && strings.Contains(l, "{") {
			end := d.blockEnd(i+1, style)
			if end > i+1 {
				classes = append(classes, cls{name: m[1], start: i + 1, end: end})
			}
		}
	}
	for i := 0; i < len(d.lines); i++ {
		am := marker.FindStringSubmatch(d.lines[i])
		if am == nil {
			continue
		}
		display, skipped := "", false
		for j := i; j < min(i+8, len(d.lines)); j++ {
			l := d.lines[j]
			if dm := displayName.FindStringSubmatch(l); dm != nil {
				display = unquote(dm[1])
			}
			if skipAnno.MatchString(l) {
				skipped = true
			}
			if j > i && (marker.MatchString(l) || strings.HasPrefix(strings.TrimSpace(l), "@") || strings.HasPrefix(strings.TrimSpace(l), "#[") || strings.HasPrefix(strings.TrimSpace(l), "[")) {
				continue
			}
			m := decl.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			name := strings.Trim(m[1], "`")
			behavior := Humanize(name)
			if display != "" {
				behavior = sentence(display)
			}
			var chain []string
			for _, c := range classes {
				if c.start <= j+1 && j+1 <= c.end && c.name != "tests" {
					chain = append(chain, c.name)
				}
			}
			d.add(TestCase{Name: name, Behavior: behavior, Kind: KindTest, Framework: framework, Line: j + 1, EndLine: d.blockEnd(j+1, style), Suite: chain, Skipped: skipped})
			i = j
			break
		}
	}
}

// methods finds tests identified by their declaration alone (XCTest,
// PHPUnit, GoogleTest).
func (d *discoverer) methods(decl *regexp.Regexp, framework string) {
	for i, l := range d.lines {
		m := decl.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		name := m[1]
		var suite []string
		if len(m) > 2 && m[2] != "" { // TEST(Suite, Name)
			suite, name = []string{m[1]}, m[2]
		}
		d.add(TestCase{Name: name, Kind: KindTest, Framework: framework, Line: i + 1, EndLine: d.blockEnd(i+1, cSyntax), Suite: suite})
	}
}

// unquote strips matching quotes (", ', `) from a literal and unescapes
// the common escapes.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'' || s[0] == '`') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return strings.NewReplacer(`\"`, `"`, `\'`, `'`, `\\`, `\`, `\n`, " ").Replace(s)
}

// sentence capitalizes the first letter of free text.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// Humanize turns a test identifier into a readable phrase:
// TestBusDropsWhenFull → "Bus drops when full",
// test_parse_empty_input → "Parse empty input",
// TestStore_AddRepo → "Store: add repo".
func Humanize(name string) string {
	name = strings.Trim(name, "`")
	if strings.ContainsAny(name, " -") {
		return sentence(strings.ReplaceAll(name, "_", " "))
	}
	for _, p := range []string{"Benchmark", "Example", "Fuzz", "Test", "test_", "test"} {
		if rest, ok := strings.CutPrefix(name, p); ok && rest != "" && (p == "test_" || !unicode.IsLower(rune(rest[0])) || mixedCaseAt([]rune(rest), 0) != "") {
			name = strings.TrimPrefix(rest, "_")
			break
		}
	}
	for _, s := range []string{"_test", "Test", "Tests"} {
		if rest, ok := strings.CutSuffix(name, s); ok && rest != "" {
			name = rest
			break
		}
	}
	segs := strings.Split(name, "_")
	snake := true
	for _, s := range segs {
		if s != strings.ToLower(s) {
			snake = false
		}
	}
	var parts []string
	for _, s := range segs {
		if s == "" {
			continue
		}
		parts = append(parts, splitCamel(s))
	}
	sep := ": "
	if snake {
		sep = " "
	}
	out := strings.Join(parts, sep)
	if mixedCaseAt([]rune(out), 0) != "" {
		return out // keep "iOS build" rather than "IOS build"
	}
	return sentence(out)
}

// mixedCase are words written with internal capitals that must not be
// split at each capital letter (JaCoCo, not "ja co co"). Longest first, so
// "JavaScript" wins over shorter overlaps.
var mixedCase = []string{
	"PostgreSQL", "JavaScript", "TypeScript", "WebSocket", "CodeDiver", "LinkedIn", "YouTube",
	"OpenAPI", "OpenSSF", "GraphQL", "GitHub", "GitLab", "DevOps", "JaCoCo", "iPhone",
	"MySQL", "MacOS", "macOS", "OAuth", "iPad", "iOS",
}

// mixedCaseAt returns the known mixed-case word starting at r[i] on a word
// boundary, or "".
func mixedCaseAt(r []rune, i int) string {
	for _, term := range mixedCase {
		t := []rune(term)
		if i+len(t) > len(r) || string(r[i:i+len(t)]) != term {
			continue
		}
		// A lowercase-initial term (iOS, macOS) only starts a word at the
		// start or after a non-lowercase character.
		startsWord := i == 0 || unicode.IsUpper(t[0]) || !unicode.IsLower(r[i-1])
		endsWord := i+len(t) == len(r) || !unicode.IsLower(r[i+len(t)])
		if startsWord && endsWord {
			return term
		}
	}
	return ""
}

// splitCamel splits camelCase into lower-case words, keeping acronyms
// (HTTPServer → "HTTP server") and known mixed-case words
// (ParseJaCoCo → "parse JaCoCo").
func splitCamel(s string) string {
	r := []rune(s)
	var words []string
	plain := 0
	for i := 0; i < len(r); {
		if term := mixedCaseAt(r, i); term != "" {
			words = append(words, camelWords(r[plain:i])...)
			words = append(words, term)
			i += len([]rune(term))
			plain = i
			continue
		}
		i++
	}
	words = append(words, camelWords(r[plain:])...)
	return strings.Join(words, " ")
}

// camelWords splits a camelCase run into lower-case words, keeping
// all-caps acronyms as they are.
func camelWords(r []rune) []string {
	var words []string
	start := 0
	for i := 1; i <= len(r); i++ {
		boundary := i == len(r) ||
			(unicode.IsUpper(r[i]) && (unicode.IsLower(r[i-1]) || unicode.IsDigit(r[i-1]))) ||
			(unicode.IsUpper(r[i]) && i+1 < len(r) && unicode.IsLower(r[i+1]) && unicode.IsUpper(r[i-1]))
		if boundary && i > start {
			words = append(words, string(r[start:i]))
			start = i
		}
	}
	for i, w := range words {
		if len(w) > 1 && strings.ToUpper(w) == w {
			continue // acronym
		}
		words[i] = strings.ToLower(w)
	}
	return words
}

// syntax describes how matchClose skips strings and comments.
type syntax struct {
	quotes       string
	lineComment  string
	blockComment [2]string
	// lifetimes treats ' as a char literal only when it closes within
	// three bytes (Rust lifetimes like 'a are not strings).
	lifetimes bool
}

var (
	cSyntax    = syntax{quotes: "\"'`", lineComment: "//", blockComment: [2]string{"/*", "*/"}}
	jsSyntax   = cSyntax
	rustSyntax = syntax{quotes: "\"'", lineComment: "//", blockComment: [2]string{"/*", "*/"}, lifetimes: true}
)

// matchClose returns the offset of the bracket closing the one at open
// ('(', '[' or '{'), skipping strings and comments; -1 if unbalanced.
func matchClose(src string, open int, st syntax) int {
	if open < 0 || open >= len(src) {
		return -1
	}
	pairs := map[byte]byte{'(': ')', '[': ']', '{': '}'}
	var stack []byte
	for i := open; i < len(src); i++ {
		c := src[i]
		switch {
		case st.lineComment != "" && strings.HasPrefix(src[i:], st.lineComment):
			if nl := strings.IndexByte(src[i:], '\n'); nl >= 0 {
				i += nl
			} else {
				return -1
			}
		case st.blockComment[0] != "" && strings.HasPrefix(src[i:], st.blockComment[0]):
			if e := strings.Index(src[i+2:], st.blockComment[1]); e >= 0 {
				i += e + 3
			} else {
				return -1
			}
		case strings.IndexByte(st.quotes, c) >= 0:
			if c == '\'' && st.lifetimes {
				// 'a' or '\n' is a char literal; anything else is a lifetime ('a).
				charLit := i+2 < len(src) && (src[i+2] == '\'' || (src[i+1] == '\\' && i+3 < len(src) && strings.IndexByte(src[i+2:min(i+12, len(src))], '\'') >= 0))
				if !charLit {
					continue
				}
			}
			j := i + 1
			for ; j < len(src); j++ {
				if src[j] == '\\' && c != '`' {
					j++
					continue
				}
				if src[j] == c {
					break
				}
				if src[j] == '\n' && c != '`' {
					break // unterminated single-line string; resync
				}
			}
			i = j
		case pairs[c] != 0:
			stack = append(stack, pairs[c])
		case c == ')' || c == ']' || c == '}':
			if len(stack) == 0 || stack[len(stack)-1] != c {
				return -1
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i
			}
		}
	}
	return -1
}
