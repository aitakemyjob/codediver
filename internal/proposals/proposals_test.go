package proposals

import (
	"os"
	"strings"
	"testing"
)

type memFS map[string]string

func (m memFS) Read(p string) (string, bool, error) {
	c, ok := m[p]
	if !ok {
		return "", false, nil
	}
	return c, true, nil
}
func (m memFS) Write(p string, b []byte) error { m[p] = string(b); return nil }
func (m memFS) Delete(p string) error {
	if _, ok := m[p]; !ok {
		return os.ErrNotExist
	}
	delete(m, p)
	return nil
}
func (m memFS) Language(string) string { return "go" }

func ptr(s string) *string { return &s }

func TestApplyEdits(t *testing.T) {
	src := "a := 1\nb := 2\nb := 2\n"
	if _, err := ApplyEdits(src, []Edit{{Old: "b := 2", New: "x"}}); err == nil || !strings.Contains(err.Error(), "occurs 2 times") {
		t.Fatalf("ambiguous edit err = %v", err)
	}
	if _, err := ApplyEdits(src, []Edit{{Old: "zzz", New: "x"}}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing edit err = %v", err)
	}
	got, err := ApplyEdits(src, []Edit{{Old: "a := 1", New: "a := 10"}, {Old: "a := 10\nb", New: "a := 10\nc"}})
	if err != nil || got != "a := 10\nc := 2\nb := 2\n" {
		t.Fatalf("edits = %q %v", got, err)
	}
	crlf := "one\r\ntwo\r\n"
	if got, err := ApplyEdits(crlf, []Edit{{Old: "one\ntwo", New: "1\n2"}}); err != nil || got != "1\r\n2\r\n" {
		t.Fatalf("crlf = %q %v", got, err)
	}
}

func TestBuildChangeValidation(t *testing.T) {
	fs := memFS{"a.go": "package a\n"}
	bad := []ChangeInput{
		{Path: ""},
		{Path: "a.go"},
		{Path: "a.go", Action: ActionCreate, NewContent: ptr("x")},
		{Path: "b.go", Action: ActionModify, NewContent: ptr("x")},
		{Path: "b.go", Action: ActionDelete},
		{Path: "a.go", NewContent: ptr("package a\n")},
		{Path: "a.go", NewContent: ptr("x"), Edits: []Edit{{Old: "a", New: "b"}}},
	}
	for i, in := range bad {
		if _, err := BuildChange(fs, in); err == nil {
			t.Errorf("case %d: expected error for %+v", i, in)
		}
	}
	c, err := BuildChange(fs, ChangeInput{Path: "new.go", NewContent: ptr("package n\n\nfunc N() {}\n")})
	if err != nil || c.Action != ActionCreate || c.Additions != 3 || c.Deletions != 0 {
		t.Fatalf("create = %+v %v", c, err)
	}
}

func TestResolveAcceptRejectConflict(t *testing.T) {
	fs := memFS{"a.go": "package a\n\nfunc A() {}\n", "b.go": "package b\n", "c.go": "package c\n"}
	var changes []*FileChange
	for _, in := range []ChangeInput{
		{Path: "a.go", Edits: []Edit{{Old: "func A() {}", New: "func A() { println() }"}}},
		{Path: "b.go", Action: ActionDelete},
		{Path: "c.go", NewContent: ptr("package c // changed\n")},
	} {
		c, err := BuildChange(fs, in)
		if err != nil {
			t.Fatal(err)
		}
		changes = append(changes, c)
	}
	if got := changes[0].ChangedNewLines(); len(got) != 1 || got[0] != 3 {
		t.Errorf("changed new lines = %v", got)
	}
	s := NewStore()
	p := s.Add(&Proposal{Repo: "r", Title: "t", Changes: changes})
	if p.ID == "" || p.Status != StatusPending {
		t.Fatalf("added = %+v", p)
	}

	// Someone edits c.go behind our back.
	fs["c.go"] = "package c // user edit\n"

	p, err := s.Resolve(fs, p.ID, []string{"a.go"}, true, "")
	if err != nil || p.Changes[0].Status != StatusApplied || !strings.Contains(fs["a.go"], "println") || p.Status != StatusPartial {
		t.Fatalf("accept a.go: %+v %v", p, err)
	}
	p, err = s.Resolve(fs, p.ID, []string{"b.go"}, false, "keep b")
	if err != nil || p.Changes[1].Status != StatusRejected || fs["b.go"] == "" || p.Feedback != "keep b" {
		t.Fatalf("reject b.go: %+v %v", p, err)
	}
	p, err = s.Resolve(fs, p.ID, nil, true, "")
	if err == nil || p.Changes[2].Status != StatusConflict || fs["c.go"] != "package c // user edit\n" || p.Status != StatusConflict {
		t.Fatalf("conflict c.go: %+v %v", p, err)
	}
	if list := s.List("r"); len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}
}
