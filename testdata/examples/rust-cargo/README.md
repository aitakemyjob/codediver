# rust-cargo

A small orders service in Rust with `cargo test`: `order` (validation and
pricing), `store::MemoryStore`, `notify::send` (a webhook that's hard to
test) and `api::Handler` (request handling). Tests sit next to the code in
`#[cfg(test)]` modules. See [../README.md](../README.md) for what to try and
what the gaps are.

Regenerate coverage (needs [cargo-llvm-cov](https://github.com/taiki-e/cargo-llvm-cov)):
`cargo llvm-cov --lcov --output-path lcov.info --remap-path-prefix`
(`--remap-path-prefix` keeps the report's paths relative; don't commit `target/`)
