package qa

import (
	"regexp"
	"strconv"
	"strings"
)

// Hint is a static observation about why a function is hard to test, with
// the conventional fix.
type Hint struct {
	Kind       string `json:"kind"`
	Line       int    `json:"line,omitempty"`
	Match      string `json:"match,omitempty"`
	Problem    string `json:"problem"`
	Suggestion string `json:"suggestion"`
}

// Hint kinds.
const (
	HintClock      = "clock"
	HintRandom     = "random"
	HintEnv        = "env"
	HintNetwork    = "network"
	HintFilesystem = "filesystem"
	HintProcess    = "process"
	HintSleep      = "sleep"
	HintExit       = "exit"
	HintGlobal     = "global"
	HintBrowser    = "browser"
	HintComplexity = "complexity"
	HintParams     = "params"
)

type hintRule struct {
	kind       string
	re         *regexp.Regexp
	problem    string
	suggestion string
}

// hintRules detect hard-wired dependencies across common languages.
var hintRules = []hintRule{
	{HintClock, regexp.MustCompile(`\btime\.(?:Now|Since|Until)\(|\bDate\.now\(|\bnew Date\(\s*\)|\bdatetime\.(?:now|utcnow|today)\(|\bdate\.today\(|\btime\.time\(|System\.currentTimeMillis|Instant\.now\(|LocalDate(?:Time)?\.now\(|DateTime\.(?:Now|UtcNow)\b|Utc::now\(|SystemTime::now\(`),
		"reads the system clock directly, so results depend on when the test runs",
		"accept the current time (or a clock interface / now func) as a parameter or field"},
	{HintRandom, regexp.MustCompile(`\brand\.\w+\(|\bMath\.random\(|\brandom\.\w+\(|\buuid\.(?:New|uuid4)|\bcrypto\.randomUUID\(|new Random\(|\bthread_rng\(`),
		"uses randomness directly, so outputs are not reproducible",
		"inject the random source or ID generator so tests can supply a fixed one"},
	{HintEnv, regexp.MustCompile(`\bos\.(?:Getenv|LookupEnv)\(|\bprocess\.env\b|\bos\.environ\b|\bos\.getenv\(|System\.getenv\(|Environment\.GetEnvironmentVariable\(|\benv::var\(`),
		"reads environment variables inside the logic",
		"read configuration once at startup and pass the values in"},
	{HintNetwork, regexp.MustCompile(`\bhttp\.(?:Get|Post|Head|PostForm|DefaultClient)\b|\bnet\.Dial|\bfetch\(|\baxios[.(]|\brequests\.(?:get|post|put|delete|patch|request)\(|\burllib\.request|\bnew HttpClient\(|\bHttpClient\.newHttpClient\(|\breqwest::`),
		"talks to the network directly",
		"depend on an interface (or pass the client/base URL in) so tests can use a fake or a local test server"},
	{HintFilesystem, regexp.MustCompile(`\bos\.(?:ReadFile|WriteFile|Open|OpenFile|Create|Remove|RemoveAll|MkdirAll|ReadDir|Stat)\(|\bioutil\.\w+\(|\bfs\.(?:readFileSync|writeFileSync|readFile|writeFile|existsSync|mkdirSync|readdirSync)\(|\bopen\([^)]*['"][rwa]b?['"]|\bFiles\.(?:read|write|newBuffered)\w*\(|\bFile\.(?:ReadAll|WriteAll)\w*\(|\bstd::fs::`),
		"reads or writes files directly",
		"take an io.Reader/io.Writer (or a filesystem interface) so tests can use in-memory data or a temp dir"},
	{HintProcess, regexp.MustCompile(`\bexec\.Command(?:Context)?\(|\bchild_process\b|\b(?:execSync|spawnSync|spawn)\(|\bsubprocess\.\w+\(|\bos\.system\(|Runtime\.getRuntime\(\)\.exec|\bProcess\.Start\(|\bCommand::new\(`),
		"runs external processes",
		"wrap command execution behind a small interface so tests can stub it"},
	{HintSleep, regexp.MustCompile(`\btime\.Sleep\(|\btime\.After\(|\bsetTimeout\(|\bsetInterval\(|\btime\.sleep\(|\bThread\.sleep\(|\bTask\.Delay\(|\bthread::sleep\(`),
		"waits on real time, making tests slow or flaky",
		"inject the timer/clock (or make the delay configurable) so tests run instantly"},
	{HintExit, regexp.MustCompile(`\bos\.Exit\(|\blog\.Fatal\w*\(|\bprocess\.exit\(|\bsys\.exit\(|\bSystem\.exit\(|\bEnvironment\.Exit\(|\bstd::process::exit\(`),
		"terminates the process, which kills the test runner",
		"return an error and let the caller (main) decide to exit"},
	{HintBrowser, regexp.MustCompile(`\bdocument\.\w+|\bwindow\.(?:location|localStorage|sessionStorage|open)\b|\blocalStorage\.\w+`),
		"touches browser globals directly",
		"pass the storage/location dependency in, or isolate DOM access in a thin adapter"},
}

// Hints scans a function body for testability problems. lines are the
// whole file's lines; start and end are the function's 1-based range.
func Hints(lines []string, start, end, complexity, params int) []Hint {
	var out []Hint
	seen := map[string]bool{}
	for ln := start; ln <= end && ln <= len(lines); ln++ {
		text := lines[ln-1]
		code := stripLineComment(text)
		for _, r := range hintRules {
			if seen[r.kind] {
				continue
			}
			if m := r.re.FindString(code); m != "" {
				seen[r.kind] = true
				out = append(out, Hint{Kind: r.kind, Line: ln, Match: strings.TrimSpace(m), Problem: r.problem, Suggestion: r.suggestion})
			}
		}
	}
	if complexity >= 15 {
		out = append(out, Hint{Kind: HintComplexity, Line: start, Problem: "has many branches (complexity " + strconv.Itoa(complexity) + "), so covering every path needs many tests",
			Suggestion: "extract the decision logic into small pure functions that can be tested one by one"})
	}
	if params >= 6 {
		out = append(out, Hint{Kind: HintParams, Line: start, Problem: "takes " + strconv.Itoa(params) + " parameters, which makes test setup verbose",
			Suggestion: "group related parameters into an options struct/object"})
	}
	return out
}

// stripLineComment removes a trailing // or # comment (best effort).
func stripLineComment(s string) string {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") {
		return ""
	}
	if i := strings.Index(s, " //"); i >= 0 {
		return s[:i]
	}
	return s
}

// Call is a name invoked in a test body, with its qualifier if any
// (pkg.Func, obj.Method, new Class).
type Call struct {
	Qual string
	Name string
}

var (
	callRe = regexp.MustCompile(`(?:\b([A-Za-z_$][\w$]*)\s*\.\s*)?\b([A-Za-z_$][\w$]*)\s*(?:<[^<>()]*>|\[[^\[\]()]*\])?\s*\(`)
	newRe  = regexp.MustCompile(`\bnew\s+([A-Z][\w$]*)`)
	// JSX elements render components: <Menu ...> exercises Menu.
	jsxRe = regexp.MustCompile(`<([A-Z][\w$]*)(?:[\s/>]|$)`)
	// Language keywords and assertion/test-framework names that are never
	// the code under test.
	notTargets = setOf(`if for while switch catch return func function fn def class new typeof sizeof await async yield
		super this self print println printf len cap append make panic recover delete copy close new_ go defer range
		expect assert require describe it test context beforeEach afterEach beforeAll afterAll setup teardown
		Run Fatal Fatalf Error Errorf Log Logf Helper Cleanup Skip Skipf Parallel TempDir Setenv Equal NoError
		toBe toEqual toBeNull toBeTruthy toHaveLength toContain toThrow render fireEvent waitFor act mock fn spyOn
		assertEqual assertTrue assertFalse assertRaises assertIn raises fixture parametrize mark
		assertEquals assertThat assertNotNull assertThrows verify when given then and should`)
	// Receivers that belong to test frameworks.
	testReceivers = setOf(`t b f tb testing require assert expect vi jest screen fireEvent userEvent cy pytest mock mocker self unittest Assert Assertions Mockito XCTAssert`)
)

// setOf builds a lookup set from whitespace-separated words.
func setOf(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// Calls extracts the distinct calls made in source text, ignoring test
// framework helpers and language keywords.
func Calls(src string) []Call {
	seen := map[Call]bool{}
	var out []Call
	addCall := func(c Call) {
		if notTargets[c.Name] || testReceivers[c.Qual] || seen[c] {
			return
		}
		seen[c] = true
		out = append(out, c)
	}
	for _, l := range strings.Split(src, "\n") {
		code := stripLineComment(l)
		for _, m := range callRe.FindAllStringSubmatch(code, -1) {
			addCall(Call{Qual: m[1], Name: m[2]})
		}
		for _, m := range newRe.FindAllStringSubmatch(code, -1) {
			addCall(Call{Name: m[1]})
		}
		for _, m := range jsxRe.FindAllStringSubmatch(code, -1) {
			addCall(Call{Name: m[1]})
		}
	}
	return out
}
