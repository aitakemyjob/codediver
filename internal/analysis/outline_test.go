package analysis

import "testing"

func TestGoOutline(t *testing.T) {
	root, _ := writeTree(t, map[string]string{"x.go": `package x

import (
	"fmt"
	"net/http"
)

// MaxItems caps a page.
const MaxItems = 50

type Color int

const (
	Red Color = iota
	Green
)

var (
	client = &http.Client{}
	_      = fmt.Sprint
	name, Version string = "x", "1.0"
)

func f() { var local = 1; _ = local }
`})
	o, err := FileOutline(root, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Imports) != 2 || o.Imports[1] != "net/http" {
		t.Errorf("imports = %v", o.Imports)
	}
	by := map[string]Variable{}
	for _, v := range o.Variables {
		by[v.Name] = v
	}
	if len(o.Variables) != 6 {
		t.Fatalf("variables = %+v", o.Variables)
	}
	if v := by["MaxItems"]; v.Kind != "const" || v.Value != "50" || v.Doc != "MaxItems caps a page." || !v.Exported || v.Line != 9 {
		t.Errorf("MaxItems = %+v", v)
	}
	if v := by["Green"]; v.Type != "Color" || v.Kind != "const" {
		t.Errorf("iota continuation should inherit type: %+v", v)
	}
	if v := by["client"]; v.Kind != "var" || v.Value != "&http.Client{}" || v.Exported {
		t.Errorf("client = %+v", v)
	}
	if v := by["Version"]; v.Type != "string" || v.Value != `"1.0"` {
		t.Errorf("Version = %+v", v)
	}
	if _, ok := by["local"]; ok {
		t.Error("function-local variables must not be listed")
	}
}

func TestGenericOutline(t *testing.T) {
	root, _ := writeTree(t, map[string]string{
		"a.ts": "import { x } from './x';\n\nexport const API_URL: string = 'http://localhost';\nlet count = 0;\nconst handler = (e) => e;\nfunction f() {\n  const inner = 1;\n}\n",
		"b.py": "import os\nfrom .util import helper\n\nTIMEOUT = 30\nname: str = 'svc'\n\ndef run():\n    local = 1\n",
	})
	ts, _ := FileOutline(root, "a.ts", "typescript")
	if len(ts.Variables) != 2 || ts.Variables[0].Name != "API_URL" || ts.Variables[0].Type != "string" || ts.Variables[0].Kind != "const" || !ts.Variables[0].Exported || ts.Variables[1].Kind != "var" {
		t.Errorf("ts vars = %+v", ts.Variables)
	}
	if len(ts.Imports) != 1 || ts.Imports[0] != "./x" {
		t.Errorf("ts imports = %v", ts.Imports)
	}
	py, _ := FileOutline(root, "b.py", "python")
	if len(py.Variables) != 2 || py.Variables[0].Kind != "const" || py.Variables[1].Type != "str" {
		t.Errorf("py vars = %+v", py.Variables)
	}
	if len(py.Imports) != 2 {
		t.Errorf("py imports = %v", py.Imports)
	}
}
