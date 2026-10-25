// Package api serves orders over HTTP.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"example.com/orders/notify"
	"example.com/orders/order"
	"example.com/orders/store"
)

// Handler serves the orders API.
type Handler struct {
	store store.Store
}

// New returns a handler backed by s.
func New(s store.Store) *Handler { return &Handler{store: s} }

// CreateOrder validates, prices, stores and announces an order.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var o order.Order
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := order.Validate(o); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := h.store.Save(o); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = notify.Send(o)
	writeJSON(w, http.StatusCreated, map[string]any{"id": o.ID, "total": order.Total(o)})
}

// GetOrder returns one order.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	o, err := h.store.Get(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// ListOrders returns the orders named in ?ids=a,b,c, largest quantity
// first.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := loadAll(h.store, strings.Split(r.URL.Query().Get("ids"), ","))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].Qty != orders[j].Qty {
			return orders[i].Qty > orders[j].Qty
		}
		return orders[i].ID < orders[j].ID
	})
	writeJSON(w, http.StatusOK, orders)
}

// loadAll loads every ID through a generic repository.
func loadAll(repo store.Repo[order.Order], ids []string) ([]order.Order, error) {
	out := make([]order.Order, 0, len(ids))
	for _, id := range ids {
		o, err := repo.Get(id)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// writeJSON writes v as a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
