package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/orders/store"
)

func TestCreateOrderRejectsBadJSON(t *testing.T) {
	h := New(store.NewMemory())
	w := httptest.NewRecorder()
	h.CreateOrder(w, httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("{")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}
