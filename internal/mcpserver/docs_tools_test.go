package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/docs"
)

func TestLanguageSupport(t *testing.T) {
	h := newHarness(t)
	// Without a language: the whole page, as shipped in this build.
	if out := h.mustCall("get_language_support", nil); out != docs.Languages {
		t.Errorf("full page differs from the embedded doc")
	}
	// One language: the general sections, its matrix row, its limitations
	// and the trace section, with the issue links still resolving.
	ts := h.mustCall("get_language_support", map[string]any{"language": "ts"}, "# Language support: TypeScript",
		"## Two analyzers", "## The `~` marker", "| **TypeScript, JavaScript** |", "### TypeScript and JavaScript",
		"**Object literals:**", "## Trace view", "[#70]: https://")
	if !strings.Contains(ts, "## Support matrix\n\n| | Symbols") {
		t.Errorf("matrix header missing:\n%s", ts)
	}
	for _, other := range []string{"| **Go** |", "### Python", "### Ruby"} {
		if strings.Contains(ts, other) {
			t.Errorf("typescript answer includes %q", other)
		}
	}
	h.mustCall("get_language_support", map[string]any{"language": "csharp"}, "| **C#** |", "### Java, Kotlin, C# and Rust", "interface method declarations")
	h.mustCall("get_language_support", map[string]any{"language": "lua"}, "| **Swift, Dart, Lua** |", "Nothing beyond the general limits")
	if out, isErr := h.call("get_language_support", map[string]any{"language": "cobol"}); !isErr || !strings.Contains(out, "one of: go, typescript") {
		t.Errorf("unknown language = %q", out)
	}

	// The same page as a resource.
	res, err := h.cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: languagesURI})
	if err != nil || len(res.Contents) != 1 || res.Contents[0].Text != docs.Languages || res.Contents[0].MIMEType != "text/markdown" {
		t.Fatalf("resource = %+v, %v", res, err)
	}
}
