package qa

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParseContextsAttributesLinesToTests(t *testing.T) {
	data := `{"meta": {}, "files": {"app/mod.py": {"executed_lines": [1, 2, 5], "contexts": {
		"1": [""],
		"2": ["tests/test_mod.py::test_add|run", "tests/test_mod.py::TestCalc::test_div[0-1]|run"],
		"5": ["tests/test_mod.py::test_add|setup", "tests/test_mod.py::test_add|run"]}}}}`
	got, err := ParseContexts([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string][]int{
		"tests/test_mod.py::test_add":           {"app/mod.py": {2, 5}},
		"tests/test_mod.py::TestCalc::test_div": {"app/mod.py": {2}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contexts = %v", got)
	}
	if _, err := ParseContexts([]byte("mode: set")); err == nil {
		t.Error("a non-JSON report should fail")
	}
}

func TestPerTestCommands(t *testing.T) {
	if got := GoTestCommand("TestSave", "store"); got != `go test -count=1 -run "^TestSave$" -coverpkg=./... -coverprofile={out}/cover.out ./store` {
		t.Errorf("go = %s", got)
	}
	if got := GoTestCommand("TestMain2", "."); !strings.HasSuffix(got, " .") {
		t.Errorf("root package = %s", got)
	}
	id := PytestNodeID("tests/test_mod.py", []string{"TestCalc"}, "test_div")
	if id != "tests/test_mod.py::TestCalc::test_div" {
		t.Errorf("node id = %s", id)
	}
	cmd := PytestCommand([]string{id})
	if !strings.Contains(cmd, "--cov-context=test") || !strings.Contains(cmd, id) || !strings.Contains(cmd, "coverage json --show-contexts -o {out}/contexts.json") {
		t.Errorf("pytest = %s", cmd)
	}
	if runtime.GOOS != "windows" && !strings.Contains(cmd, "; python") {
		t.Errorf("the report must be written even when tests fail: %s", cmd)
	}
	if NodeIDOf("|run") != "" || NodeIDOf("") != "" {
		t.Error("contexts outside tests should be ignored")
	}
	if RelTo("py", "py/tests/a.py") != "tests/a.py" || RelTo("", "a.py") != "a.py" {
		t.Error("RelTo")
	}
}
