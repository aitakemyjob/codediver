package qa

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aitakemyjob/codediver/internal/testutil"
)

func TestParseGoProfileMergesBlocksAcrossBinaries(t *testing.T) {
	data := `mode: set
example.com/m/pkg/a.go:3.20,5.2 1 1
example.com/m/pkg/a.go:6.10,8.3 2 0
example.com/m/pkg/a.go:8.3,9.2 1 1
example.com/m/pkg/a.go:6.10,8.3 2 1
example.com/m/pkg/b.go:1.1,2.2 1 0
`
	raw, err := Parse([]byte(data))
	if err != nil || raw.Format != FormatGo {
		t.Fatalf("parse = %v %v", raw, err)
	}
	a := raw.Files["example.com/m/pkg/a.go"]
	// The 6-8 block ran in the second binary, so nothing is uncovered.
	if c, tot := a.Counts(); c != 7 || tot != 7 {
		t.Fatalf("a.go counts = %d/%d hits=%v", c, tot, a.Hits)
	}
	if len(a.Partial) != 0 {
		t.Errorf("partial = %v", a.Partial)
	}
	if c, tot := raw.Files["example.com/m/pkg/b.go"].Counts(); c != 0 || tot != 2 {
		t.Errorf("b.go = %d/%d", c, tot)
	}
}

func TestParseGoProfilePartialLine(t *testing.T) {
	raw, _ := Parse([]byte("mode: count\nm/a.go:1.1,2.10 1 3\nm/a.go:2.10,2.30 1 0\n"))
	a := raw.Files["m/a.go"]
	cov, unc, part := a.Lines()
	if !reflect.DeepEqual(cov, []int{1, 2}) || len(unc) != 0 || !reflect.DeepEqual(part, []int{2}) {
		t.Fatalf("lines = %v %v %v", cov, unc, part)
	}
}

func TestLinesAreNeverNil(t *testing.T) {
	// b.go never ran: its covered and partial lists must be empty, not nil,
	// or they reach the editor as null.
	raw, _ := Parse([]byte("mode: set\nm/b.go:1.1,2.2 1 0\n"))
	cov, unc, part := raw.Files["m/b.go"].Lines()
	if cov == nil || part == nil || !reflect.DeepEqual(unc, []int{1, 2}) {
		t.Fatalf("lines = %#v %#v %#v", cov, unc, part)
	}
}

func TestParseLCOV(t *testing.T) {
	data := `TN:
SF:src/menu.ts
DA:1,5
DA:2,0
DA:3,2
BRDA:3,0,0,2
BRDA:3,0,1,0
end_of_record
SF:src/other.ts
DA:10,1
end_of_record
`
	raw, err := Parse([]byte(data))
	if err != nil || raw.Format != FormatLCOV || len(raw.Files) != 2 {
		t.Fatalf("parse = %+v %v", raw, err)
	}
	cov, unc, part := raw.Files["src/menu.ts"].Lines()
	if !reflect.DeepEqual(cov, []int{1, 3}) || !reflect.DeepEqual(unc, []int{2}) || !reflect.DeepEqual(part, []int{3}) {
		t.Fatalf("menu lines = %v %v %v", cov, unc, part)
	}
}

func TestParseCobertura(t *testing.T) {
	data := `<?xml version="1.0" ?>
<!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-04.dtd">
<coverage line-rate="0.5">
  <sources><source>/ci/work/proj</source></sources>
  <packages><package name="app"><classes>
    <class name="mod.py" filename="app/mod.py">
      <methods/>
      <lines>
        <line number="1" hits="1"/>
        <line number="4" hits="0"/>
        <line number="5" hits="3" branch="true" condition-coverage="50% (1/2)"/>
      </lines>
    </class>
  </classes></package></packages>
</coverage>`
	raw, err := Parse([]byte(data))
	if err != nil || raw.Format != FormatCobertura {
		t.Fatalf("parse = %v %v", raw, err)
	}
	if !reflect.DeepEqual(raw.Bases, []string{"/ci/work/proj"}) {
		t.Errorf("bases = %v", raw.Bases)
	}
	cov, unc, part := raw.Files["app/mod.py"].Lines()
	if !reflect.DeepEqual(cov, []int{1, 5}) || !reflect.DeepEqual(unc, []int{4}) || !reflect.DeepEqual(part, []int{5}) {
		t.Fatalf("lines = %v %v %v", cov, unc, part)
	}
}

func TestParseJaCoCo(t *testing.T) {
	data := `<?xml version="1.0"?><!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="demo"><sessioninfo id="x" start="1" dump="2"/>
<package name="com/acme"><sourcefile name="Cart.java">
<line nr="3" mi="0" ci="3" mb="0" cb="0"/>
<line nr="7" mi="2" ci="0" mb="0" cb="0"/>
<line nr="9" mi="0" ci="2" mb="1" cb="1"/>
</sourcefile></package></report>`
	raw, err := Parse([]byte(data))
	if err != nil || raw.Format != FormatJaCoCo {
		t.Fatalf("parse = %v %v", raw, err)
	}
	cov, unc, part := raw.Files["com/acme/Cart.java"].Lines()
	if !reflect.DeepEqual(cov, []int{3, 9}) || !reflect.DeepEqual(unc, []int{7}) || !reflect.DeepEqual(part, []int{9}) {
		t.Fatalf("lines = %v %v %v", cov, unc, part)
	}
}

func TestParseUnknown(t *testing.T) {
	if _, err := Parse([]byte(`{"total": {}}`)); err != ErrUnknownFormat {
		t.Fatalf("err = %v", err)
	}
}

func TestResolverAndReportMerge(t *testing.T) {
	files := map[string]bool{
		"go.mod": true, "internal/a/a.go": true, "web/src/menu.ts": true,
		"src/main/java/com/acme/Cart.java": true, "app/mod.py": true, "x/util.ts": true, "y/util.ts": true,
	}
	r := &Resolver{Root: "/home/dev/repo", Files: files, Modules: map[string]string{"example.com/m": ""}, Dirs: []string{"web"}}
	cases := map[string]string{
		"example.com/m/internal/a/a.go": "internal/a/a.go",
		"/home/dev/repo/app/mod.py":     "app/mod.py",
		"src/menu.ts":                   "web/src/menu.ts", // relative to the suite dir
		"com/acme/Cart.java":            "src/main/java/com/acme/Cart.java",
		"/ci/runner/work/app/mod.py":    "app/mod.py", // absolute path from another machine
		"util.ts":                       "",           // ambiguous
		"/etc/passwd":                   "",
		"../outside.go":                 "",
	}
	for raw, want := range cases {
		got, ok := r.Resolve(raw, nil)
		if got != want || ok != (want != "") {
			t.Errorf("Resolve(%q) = %q %v, want %q", raw, got, ok, want)
		}
	}
	if got, ok := r.Resolve("mod.py", []string{"/home/dev/repo/app"}); !ok || got != "app/mod.py" {
		t.Errorf("with base = %q %v", got, ok)
	}

	rep := NewReport()
	raw1, _ := Parse([]byte("mode: set\nexample.com/m/internal/a/a.go:1.1,3.2 1 1\nexample.com/m/internal/a/a.go:4.1,5.2 1 0\nexample.com/other/z.go:1.1,2.1 1 1\n"))
	src := rep.Add(raw1, Source{Path: "cover.out", ModTime: time.Unix(100, 0)}, r.Resolve)
	if src.Files != 1 || src.Skipped != 1 || rep.Generated.Unix() != 100 {
		t.Fatalf("source = %+v", src)
	}
	raw2, _ := Parse([]byte("SF:/home/dev/repo/internal/a/a.go\nDA:4,2\nend_of_record\n"))
	rep.Add(raw2, Source{Path: "lcov.info", ModTime: time.Unix(200, 0)}, r.Resolve)
	if c, tot := rep.Totals(); c != 4 || tot != 5 {
		t.Fatalf("totals = %d/%d", c, tot)
	}
	if rep.Generated.Unix() != 200 || len(rep.Sources) != 2 {
		t.Errorf("report = %+v", rep)
	}
}

func TestRanges(t *testing.T) {
	if got := Ranges([]int{1, 2, 3, 5, 7, 8}); got != "1-3, 5, 7-8" {
		t.Fatalf("Ranges = %q", got)
	}
	if Ranges(nil) != "" {
		t.Fatal("empty")
	}
}

func TestIsTestFile(t *testing.T) {
	yes := []string{"a/b_test.go", "src/Menu.test.tsx", "src/__tests__/x.js", "tests/test_api.py", "pkg/api_test.py",
		"src/test/java/com/AppTest.java", "CartTests.cs", "spec/user_spec.rb", "tests/integration.rs", "FooTest.php", "AppTests.swift",
		"src/__mocks__/api.ts"}
	no := []string{"a/b.go", "src/Menu.tsx", "tests/__init__.py", "tests/conftest.py", "src/main/java/App.java", "src/lib.rs", "testutil/helpers.go"}
	for _, p := range yes {
		if !IsTestFile(p) {
			t.Errorf("%s should be a test file", p)
		}
	}
	for _, p := range no {
		if IsTestFile(p) {
			t.Errorf("%s should not be a test file", p)
		}
	}
}

// behaviors returns "kind:behavior" for each case, for compact assertions.
func behaviors(tcs []TestCase) []string {
	var out []string
	for _, tc := range tcs {
		out = append(out, tc.Kind+":"+tc.Behavior)
	}
	return out
}

func TestDiscoverGo(t *testing.T) {
	src := "package x\n\nfunc TestMain(m *testing.M) {}\n\nfunc TestStore_AddRepo(t *testing.T) {\n" +
		"\tcases := []struct{ name string }{\n\t\t{name: \"empty path\"},\n\t}\n" +
		"\tt.Run(\"rejects files\", func(t *testing.T) {\n\t\tAddRepo(\"x\")\n\t})\n}\n\n" +
		"func BenchmarkIndex(b *testing.B) {}\n\nfunc testHelper() {}\n\nfunc TestSkipped(t *testing.T) { t.Skip(\"later\") }\n" +
		"\nfunc TestNeedsGit(t *testing.T) {\n\tif _, err := exec.LookPath(\"git\"); err != nil {\n\t\tt.Skip(\"no git\")\n\t}\n}\n"
	got := Discover("x/x_test.go", "go", src)
	want := []string{"test:Store: add repo", "case:Empty path", "subtest:Rejects files", "benchmark:Index", "test:Skipped", "test:Needs git"}
	if !reflect.DeepEqual(behaviors(got), want) {
		t.Fatalf("got %v\nwant %v", behaviors(got), want)
	}
	if got[0].Line != 5 || got[0].EndLine != 12 || got[2].Parent != got[0].ID || got[2].EndLine != 11 || !got[4].Skipped || got[5].Skipped {
		t.Errorf("ranges/parents = %+v", got)
	}
}

func TestDiscoverJS(t *testing.T) {
	src := `import { describe, it, expect } from 'vitest';
describe('Menu', () => {
  it('opens on click', () => {
    const s = "})"; // brackets in strings don't confuse nesting
    expect(open()).toBe(true);
  });
  describe.skip('keyboard', () => {
    test.each([[1], [2]])('closes on Escape %i', () => {});
  });
});
it.todo('handles touch');
`
	got := Discover("src/Menu.test.tsx", "typescriptreact", src)
	want := []string{"suite:Menu", "test:Opens on click", "suite:Keyboard", "test:Closes on Escape %i", "todo:Handles touch"}
	if !reflect.DeepEqual(behaviors(got), want) {
		t.Fatalf("got %v\nwant %v", behaviors(got), want)
	}
	if got[1].Framework != "vitest" || got[1].Line != 3 || got[1].EndLine != 6 || got[1].Parent != got[0].ID {
		t.Errorf("it = %+v", got[1])
	}
	if !reflect.DeepEqual(got[3].Suite, []string{"Menu", "keyboard"}) || !got[2].Skipped {
		t.Errorf("nesting = %+v", got[3])
	}
	if got[4].Parent != "" {
		t.Errorf("top-level todo has parent %q", got[4].Parent)
	}
}

func TestDiscoverPython(t *testing.T) {
	src := `import pytest

class TestCart:
    def test_total_is_zero_when_empty(self):
        """An empty cart costs nothing."""
        assert Cart().total() == 0

    @pytest.mark.skip
    def test_discounts(self):
        pass

def test_parse_empty_input():
    assert parse("") == []
`
	got := Discover("tests/test_cart.py", "python", src)
	want := []string{"suite:Cart", "test:An empty cart costs nothing", "test:Discounts", "test:Parse empty input"}
	if !reflect.DeepEqual(behaviors(got), want) {
		t.Fatalf("got %v\nwant %v", behaviors(got), want)
	}
	if got[1].Parent != got[0].ID || got[1].EndLine != 6 || !got[2].Skipped || got[3].Parent != "" || got[3].EndLine != 13 {
		t.Errorf("structure = %+v", got)
	}
}

func TestDiscoverAnnotatedLanguages(t *testing.T) {
	java := `class CartTest {
    @Test
    @DisplayName("adds an item")
    void addsItem() {
        cart.add(item);
    }

    @ParameterizedTest
    void shouldRejectNegativeQuantity(int q) throws Exception {
    }
}`
	if got := behaviors(Discover("CartTest.java", "java", java)); !reflect.DeepEqual(got, []string{"test:Adds an item", "test:Should reject negative quantity"}) {
		t.Errorf("java = %v", got)
	}
	kotlin := "class CartTest {\n  @Test\n  fun `total includes tax`() {\n  }\n}\n"
	if got := behaviors(Discover("CartTest.kt", "kotlin", kotlin)); !reflect.DeepEqual(got, []string{"test:Total includes tax"}) {
		t.Errorf("kotlin = %v", got)
	}
	rust := "fn add<'a>(x: &'a str) {}\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn adds_two_numbers() {\n        assert_eq!(add(1, 2), 3);\n    }\n}\n"
	got := Discover("src/lib.rs", "rust", rust)
	if !reflect.DeepEqual(behaviors(got), []string{"test:Adds two numbers"}) || got[0].EndLine != 7 {
		t.Errorf("rust = %+v", got)
	}
	cs := "public class CartTests {\n    [Fact]\n    public void Total_IsZero_WhenEmpty() {\n    }\n}\n"
	if got := behaviors(Discover("CartTests.cs", "csharp", cs)); !reflect.DeepEqual(got, []string{"test:Total: is zero: when empty"}) {
		t.Errorf("csharp = %v", got)
	}
	rb := "describe Cart do\n  context 'when empty' do\n    it 'costs nothing' do\n    end\n  end\nend\n"
	got = Discover("spec/cart_spec.rb", "ruby", rb)
	if !reflect.DeepEqual(behaviors(got), []string{"suite:Cart", "suite:When empty", "test:Costs nothing"}) || !reflect.DeepEqual(got[2].Suite, []string{"Cart", "when empty"}) {
		t.Errorf("ruby = %+v", got)
	}
	swift := "final class CartTests: XCTestCase {\n    func testTotal() {\n    }\n}\n"
	if got := behaviors(Discover("CartTests.swift", "swift", swift)); !reflect.DeepEqual(got, []string{"test:Total"}) {
		t.Errorf("swift = %v", got)
	}
	cpp := "TEST(CartTest, AddsItems) {\n}\n"
	if got := Discover("cart_test.cc", "cpp", cpp); len(got) != 1 || got[0].Behavior != "Adds items" || got[0].Suite[0] != "CartTest" {
		t.Errorf("cpp = %+v", got)
	}
}

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"TestBusDropsWhenFull":   "Bus drops when full",
		"test_parse_empty_input": "Parse empty input",
		"TestHTTPServerStarts":   "HTTP server starts",
		"TestStore_AddRepo":      "Store: add repo",
		"rejects files":          "Rejects files",
		"empty_path_case":        "Empty path case",
		"Testing":                "Testing",
		// Known mixed-case words stay whole (#20).
		"TestParseJaCoCo":          "Parse JaCoCo",
		"TestGitHubLoginFlow":      "GitHub login flow",
		"TestOAuthTokenRefresh":    "OAuth token refresh",
		"TestTypeScriptAndGraphQL": "TypeScript and GraphQL",
		"TestiOSBuild":             "iOS build",
		"TestBuildForMacOS":        "Build for MacOS",
		"TestBuildFor_macOS":       "Build for: macOS",
		"TestGitHubber":            "Git hubber", // not the word GitHub
		"test_parse_jacoco":        "Parse jacoco",
	}
	for in, want := range cases {
		if got := Humanize(in); got != want {
			t.Errorf("Humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHints(t *testing.T) {
	src := strings.Split(`func Sync() error {
	// time.Now() in a comment is ignored
	start := time.Now()
	if os.Getenv("DEBUG") != "" {
		log.Fatal("x")
	}
	resp, _ := http.Get(url)
	data, _ := os.ReadFile("x")
	time.Sleep(time.Second)
	return exec.Command("git").Run()
}`, "\n")
	hints := Hints(src, 1, len(src), 16, 7)
	var kinds []string
	for _, h := range hints {
		kinds = append(kinds, h.Kind)
	}
	want := []string{HintClock, HintEnv, HintExit, HintNetwork, HintFilesystem, HintSleep, HintProcess, HintComplexity, HintParams}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds = %v", kinds)
	}
	if hints[0].Line != 3 || hints[0].Suggestion == "" {
		t.Errorf("clock hint = %+v", hints[0])
	}
	// Mail, sockets and other clients count as network too.
	for _, line := range []string{"with smtplib.SMTP(host) as s:", "return smtp.SendMail(addr, a, from, to, msg)", "sock = socket.create_connection((h, p))", "r = httpx.get(url)", "ws := new WebSocket(url)"} {
		if hs := Hints([]string{line}, 1, 1, 1, 0); len(hs) != 1 || hs[0].Kind != HintNetwork {
			t.Errorf("%q hints = %+v", line, hs)
		}
	}
}

func TestCalls(t *testing.T) {
	got := Calls(`s := NewStore(path)
r, err := s.AddRepo(dir, "")
if err != nil { t.Fatal(err) }
expect(render(new Menu())).toBe(true)
assert.Equal(t, 1, pkg.Count[int](xs))
render(<Inspector open />)
const el = (<Toolbar
  open />)`, "typescript")
	want := []Call{{"", "NewStore"}, {"s", "AddRepo"}, {"", "Menu"}, {"pkg", "Count"}, {"", "Inspector"}, {"", "Toolbar"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Calls = %v", got)
	}

	// Calls inside strings and comments are not calls (#19); template
	// literal interpolations are.
	cases := []struct {
		lang, src string
		want      []Call
	}{
		{"go", "parse(\"NewStore(path)\", `s.AddRepo(dir)`) // Drop(x)\n/* Skip() */ check(1)", []Call{{"", "parse"}, {"", "check"}}},
		{"typescript", "expect(`total ${sum(xs)} of fake()`).toBe('Other()')", []Call{{"", "sum"}}},
		{"python", "assert run('''\nhelper()\n''') == \"Thing()\"  # ignore()", []Call{{"", "run"}}},
	}
	for _, c := range cases {
		if got := Calls(c.src, c.lang); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s Calls = %v, want %v", c.lang, got, c.want)
		}
	}
}

func TestDetectSuites(t *testing.T) {
	files := []string{"go.mod", "frontend/package.json", "py/pyproject.toml", "rs/Cargo.toml", "rs/crates/a/Cargo.toml", "java/pom.xml", "node_modules/x/package.json"}
	content := map[string]string{
		"frontend/package.json": `{"devDependencies": {"vitest": "^3"}}`,
		"py/pyproject.toml":     "[tool.pytest.ini_options]\n",
	}
	got := DetectSuites(files, func(p string) string { return content[p] })
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
		if !s.Enabled || s.Command == "" || s.Report == "" {
			t.Errorf("suite %+v incomplete", s)
		}
	}
	if !reflect.DeepEqual(ids, []string{"go:", "vitest:frontend", "maven:java", "pytest:py", "cargo:rs"}) {
		t.Fatalf("ids = %v", ids)
	}
	if got[0].Name != "Go · root" {
		t.Errorf("name = %q", got[0].Name)
	}
}

func TestExpandAndFindReport(t *testing.T) {
	dir := t.TempDir()
	if Expand("x {out}/a", "/tmp/o") != "x /tmp/o/a" {
		t.Fatal("plain expand")
	}
	if runtime.GOOS != "windows" && Expand("{out}", "/tmp/my dir") != "'/tmp/my dir'" {
		t.Fatal("quoted expand")
	}
	nested := filepath.Join(dir, "guid", "coverage.cobertura.xml")
	testutil.Must(t, os.MkdirAll(filepath.Dir(nested), 0o755))
	testutil.Must(t, os.WriteFile(nested, []byte("<coverage/>"), 0o644))
	if got, err := FindReport(filepath.Join(dir, "*", "coverage.cobertura.xml")); err != nil || got != nested {
		t.Fatalf("FindReport = %q %v", got, err)
	}
	if _, err := FindReport(filepath.Join(dir, "missing.out")); err == nil {
		t.Fatal("expected error for missing report")
	}
}

func TestRunStreamsOutputAndFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	var lines []string
	res, err := Run(context.Background(), t.TempDir(), "echo hello; echo '--- FAIL: TestX (0.00s)'; exit 3", func(l string) { lines = append(lines, l) })
	if err != nil || res.ExitCode != 3 || !reflect.DeepEqual(res.Failures, []string{"TestX"}) || lines[0] != "hello" {
		t.Fatalf("run = %+v %v %v", res, err, lines)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Run(ctx, t.TempDir(), "sleep 5 & sleep 5; wait", func(string) {}); err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("cancel: err=%v after %v", err, time.Since(start))
	}
}

func TestSetupHint(t *testing.T) {
	if h := SetupHint([]string{"ok", " MISSING DEPENDENCY  Cannot find dependency '@vitest/coverage-v8'"}); !strings.Contains(h, "npm install -D @vitest/coverage-v8") {
		t.Errorf("vitest hint = %q", h)
	}
	if h := SetupHint([]string{"pytest: error: unrecognized arguments: --cov"}); !strings.Contains(h, "pytest-cov") {
		t.Errorf("pytest hint = %q", h)
	}
	if SetupHint([]string{"PASS"}) != "" {
		t.Error("no hint expected")
	}
}
