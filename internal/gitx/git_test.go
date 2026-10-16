package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const sampleDiff = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,4 +1,5 @@ package main
 package main

-func main() {}
+func main() {
+	run()
+}
diff --git a/old.txt b/new.txt
similarity index 90%
rename from old.txt
rename to new.txt
diff --git a/gone.go b/gone.go
deleted file mode 100644
--- a/gone.go
+++ /dev/null
@@ -1 +0,0 @@
-package gone
diff --git a/img.png b/img.png
new file mode 100644
Binary files /dev/null and b/img.png differ
`

func TestParseUnifiedDiff(t *testing.T) {
	files := ParseUnifiedDiff(sampleDiff)
	if len(files) != 4 {
		t.Fatalf("got %d files: %+v", len(files), files)
	}
	m := files[0]
	if m.Path() != "main.go" || m.Status != StatusModified || m.Additions != 3 || m.Deletions != 1 {
		t.Fatalf("modified file = %+v", m)
	}
	h := m.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 4 || h.NewStart != 1 || h.NewLines != 5 || h.Header != "package main" {
		t.Fatalf("hunk header = %+v", h)
	}
	if got := m.ChangedNewLines(); len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Fatalf("changed lines = %v", got)
	}
	if r := files[1]; r.Status != StatusRenamed || r.OldPath != "old.txt" || r.NewPath != "new.txt" {
		t.Fatalf("rename = %+v", r)
	}
	if d := files[2]; d.Status != StatusDeleted || d.Path() != "gone.go" || d.Deletions != 1 {
		t.Fatalf("delete = %+v", d)
	}
	if b := files[3]; !b.Binary || b.Status != StatusAdded || b.Path() != "img.png" {
		t.Fatalf("binary = %+v", b)
	}
}

func TestRenameTarget(t *testing.T) {
	for in, want := range map[string]string{
		"a/{b => c}/d.go":  "a/c/d.go",
		"{old => new}.go":  "new.go",
		"x.go => y.go":     "y.go",
		"a/{ => sub}/f.go": "a/sub/f.go",
		"plain.go":         "plain.go",
	} {
		if got := renameTarget(in); got != want {
			t.Errorf("renameTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseStatusZ(t *testing.T) {
	out := " M a.go\x00?? new.go\x00R  b.go\x00a.go.orig\x00A  c.go\x00"
	e := parseStatusZ(out)
	if len(e) != 4 {
		t.Fatalf("entries = %+v", e)
	}
	if e[0].Status != StatusModified || e[0].Staged {
		t.Errorf("e0 = %+v", e[0])
	}
	if e[1].Status != StatusUntracked {
		t.Errorf("e1 = %+v", e[1])
	}
	if e[2].Status != StatusRenamed || e[2].OrigPath != "a.go.orig" || !e[2].Staged {
		t.Errorf("e2 = %+v", e[2])
	}
	if e[3].Status != StatusAdded {
		t.Errorf("e3 = %+v", e[3])
	}
}

func gitInit(t *testing.T) string {
	t.Helper()
	if !Available() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("a.go", "package a\n\nfunc A() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "first")
	write("a.go", "package a\n\nfunc A() { B() }\n\nfunc B() {}\n")
	run("commit", "-q", "-am", "second")
	write("a.go", "package a\n\nfunc A() { B(); B() }\n\nfunc B() {}\n")
	write("u.txt", "untracked\n")
	return dir
}

func TestRepoIntegration(t *testing.T) {
	dir := gitInit(t)
	r := New(dir)
	ctx := context.Background()

	info, err := r.Info(ctx)
	if err != nil || !info.IsRepo || info.Branch != "main" || info.Dirty != 2 {
		t.Fatalf("info = %+v %v", info, err)
	}
	diffs, err := r.Diff(ctx, DiffSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 2 || diffs[0].Path() != "a.go" || diffs[1].Status != StatusUntracked {
		t.Fatalf("working diff = %+v", diffs)
	}
	between, err := r.Diff(ctx, DiffSpec{Base: "HEAD~1", Head: "HEAD"})
	if err != nil || len(between) != 1 || between[0].Additions != 3 {
		t.Fatalf("ref diff = %+v %v", between, err)
	}
	old, err := r.Show(ctx, "HEAD~1", "a.go")
	if err != nil || old != "package a\n\nfunc A() {}\n" {
		t.Fatalf("show = %q %v", old, err)
	}
	log, err := r.Log(ctx, 10, "", "")
	if err != nil || len(log) != 2 || log[0].Subject != "second" || log[0].Files[0].Path != "a.go" {
		t.Fatalf("log = %+v %v", log, err)
	}
	churn, err := r.Churn(ctx, 30)
	if err != nil || churn["a.go"] == nil || churn["a.go"].Commits != 2 || churn["a.go"].Authors[0] != "Test" {
		t.Fatalf("churn = %+v %v", churn, err)
	}
	refs, err := r.Refs(ctx)
	if err != nil || len(refs) != 1 || refs[0].Name != "main" {
		t.Fatalf("refs = %+v %v", refs, err)
	}
	if _, err := r.Diff(ctx, DiffSpec{Base: "--output=/tmp/x"}); err == nil {
		t.Fatal("option-looking refs must be rejected")
	}
}

func TestNotARepo(t *testing.T) {
	if !Available() {
		t.Skip("git not installed")
	}
	info, err := New(t.TempDir()).Info(context.Background())
	if err != nil || info.IsRepo {
		t.Fatalf("info = %+v %v", info, err)
	}
}
