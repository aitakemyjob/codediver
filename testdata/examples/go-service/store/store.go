// Package store keeps orders.
package store

import (
	"errors"
	"sync"

	"example.com/orders/order"
)

// ErrNotFound is returned for unknown order IDs.
var ErrNotFound = errors.New("order not found")

// Store saves and loads orders.
type Store interface {
	Save(o order.Order) error
	Get(id string) (order.Order, error)
}

// Memory is an in-memory Store.
type Memory struct {
	mu     sync.Mutex
	orders map[string]order.Order
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{orders: map[string]order.Order{}} }

// Save stores an order, replacing one with the same ID.
func (m *Memory) Save(o order.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[o.ID] = o
	return nil
}

// Get loads an order by ID.
func (m *Memory) Get(id string) (order.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return order.Order{}, ErrNotFound
	}
	return o, nil
}
