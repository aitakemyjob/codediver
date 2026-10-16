package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Variable is a top-level variable or constant declared in a file.
type Variable struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // var | const
	Type      string `json:"type,omitempty"`
	Value     string `json:"value,omitempty"`
	Line      int    `json:"line"`
	Doc       string `json:"doc,omitempty"`
	Exported  bool   `json:"exported"`
	Heuristic bool   `json:"heuristic,omitempty"`
}

// Outline holds file-level facts that the symbol index does not track.
type Outline struct {
	Variables []Variable `json:"variables"`
	Imports   []string   `json:"imports"`
}

const maxValueLen = 60

// clip collapses whitespace and truncates long initializer values.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxValueLen {
		return s[:maxValueLen-1] + "…"
	}
	return s
}

// FileOutline extracts top-level variables, constants and imports.
func FileOutline(root, file, lang string) (*Outline, error) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	if lang == "go" {
		return goOutline(src)
	}
	return genericOutline(string(src), lang), nil
}

// goOutline extracts imports and package-level vars and consts from Go
// source, including iota-style constants that inherit their type.
func goOutline(src []byte) (*Outline, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if f == nil {
		return nil, err
	}
	o := &Outline{Variables: []Variable{}, Imports: []string{}}
	for _, im := range f.Imports {
		o.Imports = append(o.Imports, strings.Trim(im.Path.Value, `"`))
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
			continue
		}
		var lastType string
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			typ := ""
			if vs.Type != nil {
				typ = types.ExprString(vs.Type)
				lastType = typ
			} else if gd.Tok == token.CONST && len(vs.Values) == 0 {
				typ = lastType // iota continuation inherits the type
			}
			doc := vs.Doc
			if doc == nil && len(gd.Specs) == 1 {
				doc = gd.Doc
			}
			for i, n := range vs.Names {
				if n.Name == "_" {
					continue
				}
				v := Variable{Name: n.Name, Kind: gd.Tok.String(), Type: typ, Line: fset.Position(n.Pos()).Line, Exported: n.IsExported()}
				if i < len(vs.Values) {
					v.Value = clip(types.ExprString(vs.Values[i]))
				}
				if doc != nil {
					v.Doc = firstLine(strings.TrimSpace(doc.Text()))
				} else if vs.Comment != nil {
					v.Doc = strings.TrimSpace(vs.Comment.Text())
				}
				o.Variables = append(o.Variables, v)
			}
		}
	}
	return o, nil
}

type varPattern struct {
	re    *regexp.Regexp
	kind  func(m []string) string
	name  int
	typ   int
	value int
}

var varPatterns = map[string][]varPattern{
	"typescript": jsVars(), "javascript": jsVars(),
	"python": {{re: rx(`^([A-Za-z_]\w*)\s*(?::\s*([^=]+?))?\s*=\s*(.+)$`), name: 1, typ: 2, value: 3,
		kind: func(m []string) string {
			if strings.ToUpper(m[1]) == m[1] {
				return "const"
			}
			return "var"
		}}},
	"rust": {{re: rx(`^(?:pub(?:\([^)]*\))?\s+)?(const|static)\s+(?:mut\s+)?([A-Za-z_]\w*)\s*:\s*([^=]+?)\s*=\s*(.+?);?$`), name: 2, typ: 3, value: 4,
		kind: func(m []string) string {
			if m[1] == "const" {
				return "const"
			}
			return "var"
		}}},
}

// jsVars matches top-level const/let/var declarations in JS and TS.
func jsVars() []varPattern {
	return []varPattern{{
		re:   rx(`^(?:export\s+)?(const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::\s*([^=]+?))?\s*=\s*(.+?);?$`),
		name: 2, typ: 3, value: 4,
		kind: func(m []string) string {
			if m[1] == "const" {
				return "const"
			}
			return "var"
		},
	}}
}

// genericOutline finds unindented (top-level) declarations; values that are
// functions are skipped because the symbol index already lists them.
func genericOutline(src, lang string) *Outline {
	o := &Outline{Variables: []Variable{}, Imports: []string{}}
	sp := specs[lang]
	if sp == nil {
		return o
	}
	masked := strings.Split(maskSource(src, sp), "\n")
	raw := strings.Split(src, "\n")
	seen := map[string]bool{}
	for i, line := range raw {
		for _, re := range sp.imports {
			if m := re.FindStringSubmatch(line); m != nil && !seen[m[1]] {
				seen[m[1]] = true
				o.Imports = append(o.Imports, m[1])
			}
		}
		if i >= len(masked) || indentOf(masked[i]) != 0 {
			continue
		}
		for _, p := range varPatterns[lang] {
			m := p.re.FindStringSubmatch(strings.TrimRight(line, " \t"))
			if m == nil {
				continue
			}
			value := strings.TrimSpace(m[p.value])
			if strings.Contains(value, "=>") || strings.HasPrefix(value, "function") || strings.HasPrefix(value, "async ") || strings.HasPrefix(value, "lambda") {
				break // a function: already in the symbol index
			}
			v := Variable{Name: m[p.name], Kind: p.kind(m), Line: i + 1, Value: clip(value), Heuristic: true, Exported: isExportedGuess(lang, m[p.name], line)}
			if p.typ > 0 && p.typ < len(m) {
				v.Type = strings.TrimSpace(m[p.typ])
			}
			if d := docAbove(raw, i, sp); d != "" && sp.block != "indent" {
				v.Doc = firstLine(d)
			}
			o.Variables = append(o.Variables, v)
			break
		}
	}
	return o
}
