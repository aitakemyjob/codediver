# go-service

A small orders API in Go: `order` (validation and pricing), `store`
(in-memory), `notify` (a webhook that's hard to test) and `api` (HTTP
handlers), wired up in `cmd/orders`. See [../README.md](../README.md) for
what to try and what the gaps are.

For control flow and generics: `api.ListOrders` loads orders through
`loadAll(repo store.Repo[order.Order], …)`, a generic interface that
`Memory` satisfies, and sorts them with a `sort.Slice` closure; and
`notify.Send` retries a failed post with `goto retry`. Open **Logic** on
`ListOrders` and click the closure chip, open it on `Send` to see the
`goto` loop back, or **Trace to here** on `Memory.Get` to follow the
generic call.

Regenerate coverage: `go test -coverprofile=coverage.out -coverpkg=./... ./...`
