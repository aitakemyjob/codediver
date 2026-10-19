package qa

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Suite is a test command that produces a coverage report. Commands are
// only ever run when the developer clicks Run in CodeDiver; agents can't
// start them.
type Suite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Dir is the repo-relative working directory ("" for the root).
	Dir string `json:"dir"`
	// Command runs through the system shell. {out} expands to a scratch
	// directory outside the repository.
	Command string `json:"command"`
	// Report is the coverage file the command writes, relative to Dir. It
	// may use {out} and a * wildcard in its file name.
	Report string `json:"report"`
	Tool   string `json:"tool"`
	// Enabled suites run when the developer clicks "Run coverage".
	Enabled bool `json:"enabled"`
}

// DetectSuites proposes a coverage suite for each project found in the
// repository (go.mod, package.json with Vitest/Jest, Python, Cargo, Maven,
// Gradle, .NET). read returns a file's content, or "" if unreadable.
func DetectSuites(files []string, read func(string) string) []Suite {
	var out []Suite
	add := func(s Suite) {
		s.ID = s.Tool + ":" + s.Dir
		s.Enabled = true
		if s.Dir == "." {
			s.Dir = ""
			s.ID = s.Tool + ":"
		}
		for _, x := range out {
			if x.ID == s.ID {
				return
			}
		}
		label := s.Dir
		if label == "" {
			label = "root"
		}
		s.Name = s.Name + " · " + label
		out = append(out, s)
	}
	py := map[string]bool{}
	cargoRoots := map[string]bool{}
	for _, f := range files {
		if strings.Count(f, "/") > 3 || strings.Contains(f, "node_modules/") || InTestdata(f) {
			continue
		}
		dir, base := path.Dir(f), path.Base(f)
		switch {
		case base == "go.mod":
			add(Suite{Name: "Go", Dir: dir, Tool: "go", Command: "go test -coverprofile={out}/cover.out -coverpkg=./... ./...", Report: "{out}/cover.out"})
		case base == "package.json":
			pkg := read(f)
			switch {
			case strings.Contains(pkg, `"vitest"`):
				add(Suite{Name: "Vitest", Dir: dir, Tool: "vitest", Command: "npx vitest run --coverage.enabled --coverage.reporter=lcov --coverage.reportsDirectory={out}", Report: "{out}/lcov.info"})
			case strings.Contains(pkg, `"jest"`):
				add(Suite{Name: "Jest", Dir: dir, Tool: "jest", Command: "npx jest --coverage --coverageReporters=lcov --coverageDirectory={out}", Report: "{out}/lcov.info"})
			}
		case base == "pytest.ini" || base == "conftest.py" || base == "pyproject.toml" || base == "setup.cfg" || base == "tox.ini":
			if base == "pyproject.toml" || base == "setup.cfg" || base == "tox.ini" {
				if c := read(f); !strings.Contains(c, "pytest") {
					continue
				}
			}
			if !py[dir] {
				py[dir] = true
				add(Suite{Name: "pytest", Dir: dir, Tool: "pytest", Command: "python -m pytest --cov --cov-report=lcov:{out}/coverage.lcov", Report: "{out}/coverage.lcov"})
			}
		case base == "Cargo.toml":
			// Only the outermost Cargo project; workspaces cover members.
			inner := false
			for r := range cargoRoots {
				if r == "." || strings.HasPrefix(dir, r+"/") {
					inner = true
				}
			}
			if !inner {
				cargoRoots[dir] = true
				add(Suite{Name: "Cargo", Dir: dir, Tool: "cargo", Command: "cargo llvm-cov --lcov --output-path {out}/lcov.info", Report: "{out}/lcov.info"})
			}
		case base == "pom.xml":
			add(Suite{Name: "Maven", Dir: dir, Tool: "maven", Command: "mvn -q org.jacoco:jacoco-maven-plugin:prepare-agent test org.jacoco:jacoco-maven-plugin:report", Report: "target/site/jacoco/jacoco.xml"})
		case base == "build.gradle" || base == "build.gradle.kts":
			wrapper := "gradle"
			if hasFile(files, path.Join(dir, "gradlew")) {
				wrapper = "./gradlew"
				if runtime.GOOS == "windows" {
					wrapper = "gradlew.bat"
				}
			}
			add(Suite{Name: "Gradle", Dir: dir, Tool: "gradle", Command: wrapper + " test jacocoTestReport", Report: "build/reports/jacoco/test/jacocoTestReport.xml"})
		case strings.HasSuffix(base, ".sln"):
			add(Suite{Name: ".NET", Dir: dir, Tool: "dotnet", Command: `dotnet test --collect:"XPlat Code Coverage" --results-directory {out}`, Report: "{out}/*/coverage.cobertura.xml"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out
}

// InTestdata reports whether p lies in a testdata directory: fixtures and
// example projects that belong to the tests, not the project's own code
// under test.
func InTestdata(p string) bool {
	return strings.HasPrefix(p, "testdata/") || strings.Contains(p, "/testdata/")
}

// hasFile reports whether files contains p.
func hasFile(files []string, p string) bool {
	for _, f := range files {
		if f == p {
			return true
		}
	}
	return false
}

// KnownReports are paths where tools commonly leave coverage reports,
// checked (relative to each project directory) to import existing results
// without running anything.
var KnownReports = []string{
	"coverage.out", "cover.out", "coverage.txt", "c.out", "profile.cov", "coverage.cov",
	"coverage/lcov.info", "lcov.info", "coverage.lcov",
	"coverage.xml", "cobertura.xml", "coverage/cobertura-coverage.xml",
	"target/site/jacoco/jacoco.xml", "build/reports/jacoco/test/jacocoTestReport.xml",
}

// Expand substitutes {out} in s with a shell-quoted scratch directory.
func Expand(s, out string) string {
	return strings.ReplaceAll(s, "{out}", shellQuote(out))
}

// ExpandPath substitutes {out} in a report path without quoting.
func ExpandPath(s, out string) string { return strings.ReplaceAll(s, "{out}", out) }

// shellQuote quotes a path for the platform shell when needed.
func shellQuote(p string) string {
	if !strings.ContainsAny(p, " \t'\"&;$()") {
		return p
	}
	if runtime.GOOS == "windows" {
		return `"` + p + `"`
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

// FindReport resolves a report path that may contain a * in its last
// segment (searched recursively below the fixed prefix). It returns the
// most recently modified match.
func FindReport(p string) (string, error) {
	if !strings.Contains(p, "*") {
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}
	star := strings.Index(p, "*")
	root := filepath.Dir(p[:star+1])
	pattern := filepath.Base(p)
	var best string
	var bestTime int64
	err := filepath.WalkDir(root, func(fp string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match(pattern, d.Name()); ok {
			if st, err := d.Info(); err == nil && st.ModTime().UnixNano() > bestTime {
				best, bestTime = fp, st.ModTime().UnixNano()
			}
		}
		return nil
	})
	if best == "" {
		if err == nil {
			err = os.ErrNotExist
		}
		return "", err
	}
	return best, nil
}

// setupHints recognize common "coverage isn't set up" failures in a run's
// output and say how to fix them.
var setupHints = []struct {
	re   *regexp.Regexp
	hint string
}{
	{regexp.MustCompile(`@vitest/coverage-(v8|istanbul)`), "Vitest needs a coverage provider: npm install -D @vitest/coverage-v8 (matching your vitest version)"},
	{regexp.MustCompile(`unrecognized arguments: --cov|no module named pytest_cov`), "pytest needs the coverage plugin: pip install pytest-cov"},
	{regexp.MustCompile(`No module named pytest`), "pytest is not installed in this Python: pip install pytest pytest-cov (or point the command at your virtualenv's python)"},
	{regexp.MustCompile("no such command: `?llvm-cov"), "install cargo-llvm-cov: cargo install cargo-llvm-cov"},
	{regexp.MustCompile(`Task 'jacocoTestReport' not found`), "apply the Gradle jacoco plugin (plugins { jacoco }) so jacocoTestReport exists"},
	{regexp.MustCompile(`Cannot find module 'jest'|jest: not found`), "Jest is not installed: npm install -D jest"},
	{regexp.MustCompile(`(?i)(command not found|is not recognized as an internal or external command|executable file not found)`), "the test tool isn't on PATH for CodeDiver; use its full path in the suite command"},
}

// SetupHint returns advice for a run whose output shows a missing tool or
// plugin, or "".
func SetupHint(lines []string) string {
	for _, l := range lines {
		for _, h := range setupHints {
			if h.re.MatchString(l) {
				return h.hint
			}
		}
	}
	return ""
}

// RunResult is the outcome of running one suite.
type RunResult struct {
	ExitCode int
	// Failures are names of failed tests recognized in the output.
	Failures []string
}

var failureRes = []*regexp.Regexp{
	regexp.MustCompile(`^\s*--- FAIL: (\S+)`),                                     // go test
	regexp.MustCompile(`^FAILED (\S+?)(?:\s+-|$)`),                                // pytest
	regexp.MustCompile(`^\s*(?:×|✕|✗|FAIL)\s+(?:\S+\s+>\s+)?(.+?)(?:\s+\d+ms)?$`), // vitest/jest
	regexp.MustCompile(`^test (\S+) \.\.\. FAILED`),                               // cargo
}

// Run executes a suite's command through the system shell in dir,
// streaming each output line to onLine. A non-zero exit (usually failing
// tests) is reported in the result, not as an error; errors mean the
// command could not run or was cancelled.
func Run(ctx context.Context, dir, command string, onLine func(string)) (RunResult, error) {
	var res RunResult
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = dir
	// Keep runners non-interactive and their output plain.
	cmd.Env = append(os.Environ(), "CI=1", "NO_COLOR=1", "FORCE_COLOR=0", "TERM=dumb")
	setProcessGroup(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		return res, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			for _, re := range failureRes {
				if m := re.FindStringSubmatch(line); m != nil {
					res.Failures = append(res.Failures, strings.TrimSpace(m[1]))
					break
				}
			}
			onLine(line)
		}
		io.Copy(io.Discard, pr)
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killProcessGroup(cmd)
		<-done
		err = ctx.Err()
	}
	pw.Close()
	wg.Wait()
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		res.ExitCode = exit.ExitCode()
		return res, nil
	}
	return res, err
}
