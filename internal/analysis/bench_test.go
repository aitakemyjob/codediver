package analysis

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkBuildRealRepo analyzes the directory in CODEDIVER_BENCH_DIR:
//
//	CODEDIVER_BENCH_DIR=~/src/some-repo go test -bench BuildRealRepo -benchtime 3x ./internal/analysis
func BenchmarkBuildRealRepo(b *testing.B) {
	root := os.Getenv("CODEDIVER_BENCH_DIR")
	if root == "" {
		b.Skip("set CODEDIVER_BENCH_DIR")
	}
	var files []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	lang := func(p string) string {
		switch {
		case strings.HasSuffix(p, ".go"):
			return "go"
		case strings.HasSuffix(p, ".ts"), strings.HasSuffix(p, ".tsx"):
			return "typescript"
		case strings.HasSuffix(p, ".js"):
			return "javascript"
		case strings.HasSuffix(p, ".py"):
			return "python"
		}
		return "plaintext"
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix := Build(root, files, lang)
		st := ix.Stats()
		b.ReportMetric(float64(st.Functions), "funcs")
		b.ReportMetric(float64(st.Edges), "edges")
	}
}
