# dotnet-xunit

A small orders service in C# with xUnit: `Orders` (validation and pricing),
an `IStore` interface and the `MemoryStore` implementing it, `Notifier` (a webhook that's hard to test) and
`OrderHandler` (request handling, through `IStore`), in `src/Orders`, with tests in
`tests/Orders.Tests`. See [../README.md](../README.md) for what to try and
what the gaps are.

Regenerate coverage (coverlet runs as the `coverlet.collector` package):
`dotnet test --collect:"XPlat Code Coverage" --results-directory coverage`,
then move the produced `coverage/<guid>/coverage.cobertura.xml` to
`coverage/coverage.cobertura.xml` and make its `<source>` relative
(see the examples README).
