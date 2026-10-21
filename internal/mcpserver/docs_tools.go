package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/docs"
)

// languagesURI is the MCP resource serving the language support page.
const languagesURI = "codediver://docs/languages"

// languageNames maps the language ids CodeDiver uses (and common aliases)
// to how LANGUAGES.md names them in the support matrix.
var languageNames = map[string]string{
	"go": "Go", "golang": "Go",
	"typescript": "TypeScript", "ts": "TypeScript", "tsx": "TypeScript", "javascript": "JavaScript", "js": "JavaScript", "jsx": "JavaScript",
	"python": "Python", "py": "Python", "java": "Java", "kotlin": "Kotlin", "scala": "Scala",
	"csharp": "C#", "c#": "C#", "cs": "C#", "rust": "Rust", "rs": "Rust", "ruby": "Ruby", "rb": "Ruby", "php": "PHP",
	"c": "C", "cpp": "C++", "c++": "C++", "swift": "Swift", "dart": "Dart", "lua": "Lua",
}

// languageIDs lists the ids get_language_support accepts, for errors.
const languageIDs = "go, typescript, javascript, python, java, kotlin, scala, csharp, rust, ruby, php, c, cpp, swift, dart, lua"

// generalSections apply to every language and lead each language's answer.
var generalSections = []string{"Two analyzers", "The `~` marker"}

type languageArgs struct {
	Language string `json:"language,omitempty" jsonschema:"language to describe, e.g. typescript, python, go; omit for every language"`
}

// registerDocs serves the language support page as a tool and a resource.
func (s *Server) registerDocs() {
	addTool(s, &mcp.Tool{Name: "get_language_support", Description: "How precise CodeDiver's analysis is for a language: how calls are matched (type-checked for Go, by name elsewhere), what the ~ marker guarantees, which features exist (control flow, dependencies, tests, coverage) and the known gaps with examples. Check it before relying on a call graph, trace or blast radius outside Go.", Annotations: readOnly("Language support")},
		func(a languageArgs) string { return a.Language }, func(_ context.Context, a languageArgs) (string, error) {
			if strings.TrimSpace(a.Language) == "" {
				return docs.Languages, nil
			}
			return languageSupport(docs.Languages, a.Language)
		})
	s.srv.AddResource(&mcp.Resource{URI: languagesURI, Name: "language-support", Title: "Language support", MIMEType: "text/markdown",
		Description: "Per-language analysis precision and known gaps (the get_language_support page)."},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: languagesURI, MIMEType: "text/markdown", Text: docs.Languages}}}, nil
		})
}

// languageSupport extracts what applies to one language from the page:
// the general sections, its support matrix row, its known limitations and
// the trace section.
func languageSupport(page, lang string) (string, error) {
	name, ok := languageNames[strings.ToLower(strings.TrimSpace(lang))]
	if !ok {
		return "", fmt.Errorf("unknown language %q; one of: %s", lang, languageIDs)
	}
	sections := splitSections(page)
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Language support: %s\n\n", name)
	for _, h := range generalSections {
		sb.WriteString(sections["## "+h])
	}
	// The matrix header and this language's row.
	matrix := strings.Split(sections["## Support matrix"], "\n")
	var rows []string
	for i, l := range matrix {
		if !strings.HasPrefix(l, "|") {
			continue
		}
		if len(rows) < 2 && i+1 < len(matrix) {
			rows = append(rows, l) // header and separator
			continue
		}
		cells := strings.Split(l, "|")
		if len(cells) > 1 && hasName(cells[1], name) {
			rows = append(rows, l)
		}
	}
	sb.WriteString("## Support matrix\n\n" + strings.Join(rows, "\n") + "\n\n")
	limits := ""
	for h, body := range sections {
		if strings.HasPrefix(h, "### ") && hasName(strings.TrimPrefix(h, "### "), name) {
			limits = body
		}
	}
	if limits == "" {
		limits = "Nothing beyond the general limits of lexical analysis above is documented for " + name + ".\n\n"
	}
	sb.WriteString("## Known limitations\n\n" + limits)
	sb.WriteString(sections["## Trace view and `trace_to`"])
	// Reference-style link definitions at the end of the page.
	for _, l := range strings.Split(page, "\n") {
		if strings.HasPrefix(l, "[#") && strings.Contains(l, "]: ") {
			sb.WriteString(l + "\n")
		}
	}
	return sb.String(), nil
}

// splitSections maps each "## " and "### " heading line to its text up to
// the next heading of the same or a higher level.
func splitSections(page string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(page, "\n")
	level := func(l string) int {
		switch {
		case strings.HasPrefix(l, "### "):
			return 3
		case strings.HasPrefix(l, "## "):
			return 2
		}
		return 0
	}
	inFence := false
	fence := make([]bool, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		fence[i] = inFence
	}
	for i, l := range lines {
		lv := level(l)
		if lv == 0 || fence[i] {
			continue
		}
		j := i + 1
		for ; j < len(lines); j++ {
			if n := level(lines[j]); n != 0 && n <= lv && !fence[j] {
				break
			}
			if strings.HasPrefix(lines[j], "[#") && strings.Contains(lines[j], "]: ") {
				break
			}
		}
		out[l] = strings.Join(lines[i:j], "\n") + "\n"
	}
	return out
}

// hasName reports whether a matrix cell or heading ("**Java, Kotlin,
// Scala**", "TypeScript and JavaScript") names the language.
func hasName(text, name string) bool {
	text = strings.NewReplacer("*", "", " and ", ", ").Replace(text)
	for _, part := range strings.Split(text, ",") {
		if strings.TrimSpace(part) == name {
			return true
		}
	}
	return false
}
