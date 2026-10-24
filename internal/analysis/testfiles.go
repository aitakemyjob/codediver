package analysis

import (
	"path"
	"strings"
)

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
