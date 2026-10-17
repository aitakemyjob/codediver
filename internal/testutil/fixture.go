// Package testutil builds throwaway repositories for tests.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Files is a small but realistic Go service used across tests.
var Files = map[string]string{
	"go.mod":    "module example.com/shop\n\ngo 1.22\n",
	"README.md": "# shop\n\nA tiny example service.\n",
	"cmd/shop/main.go": `package main

import (
	"log"
	"net/http"

	"example.com/shop/api"
	"example.com/shop/store"
)

func main() {
	s := store.NewMemory()
	h := api.New(s)
	http.HandleFunc("/orders", h.CreateOrder)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
`,
	"api/api.go": `package api

import (
	"encoding/json"
	"net/http"

	"example.com/shop/store"
)

// Handler serves the HTTP API.
type Handler struct {
	store store.Store
}

func New(s store.Store) *Handler { return &Handler{store: s} }

// CreateOrder validates and saves an order.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var o store.Order
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validate(o); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := h.store.Save(o); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func validate(o store.Order) error {
	if o.Qty <= 0 {
		return store.ErrInvalid
	}
	return nil
}
`,
	"store/store.go": `package store

import "errors"

var ErrInvalid = errors.New("invalid order")

type Order struct {
	ID  string
	Qty int
}

// Store persists orders.
type Store interface {
	Save(o Order) error
}

type Memory struct {
	orders map[string]Order
}

func NewMemory() *Memory { return &Memory{orders: map[string]Order{}} }

// Save stores an order in memory.
func (m *Memory) Save(o Order) error {
	m.orders[o.ID] = o
	return nil
}
`,
	"web/app.ts": `import { post } from './http';

export async function submit(qty: number) {
  return post('/orders', { qty });
}
`,
	"web/http.ts": `export function post(url: string, body: unknown) {
  return fetch(url, { method: 'POST', body: JSON.stringify(body) });
}
`,
	".env": "SECRET=hunter2\n",
}

// Repo writes Files into a temp dir and, when git is available, commits them.
func Repo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	for name, body := range Files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := exec.LookPath("git"); err == nil {
		Git(t, dir, "init", "-q", "-b", "main")
		Git(t, dir, "add", ".")
		Git(t, dir, "commit", "-q", "-m", "initial")
	}
	return dir
}

// Git runs a git command in dir with a fixed identity.
func Git(t testing.TB, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Must fails the test immediately when a setup step returns an error.
func Must(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}
