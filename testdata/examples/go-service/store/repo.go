package store

// Repo loads values of any type by ID. Store satisfies Repo[order.Order],
// so a call through a Repo[order.Order] reaches Memory.Get.
type Repo[T any] interface {
	Get(id string) (T, error)
}
