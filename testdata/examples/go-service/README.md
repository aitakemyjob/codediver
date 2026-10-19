# go-service

A small orders API in Go: `order` (validation and pricing), `store`
(in-memory), `notify` (a webhook that's hard to test) and `api` (HTTP
handlers), wired up in `cmd/orders`. See [../README.md](../README.md) for
what to try and what the gaps are.

Regenerate coverage: `go test -coverprofile=coverage.out -coverpkg=./... ./...`
