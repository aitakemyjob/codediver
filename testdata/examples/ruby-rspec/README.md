# ruby-rspec

A small orders service in Ruby with RSpec: `Orders` (validation and
pricing), `Orders::MemoryStore`, `Orders::Notifier` (a webhook that's hard
to test) and `Orders::OrderHandler` (request handling). SimpleCov writes
LCOV through `simplecov-lcov` (configured in `spec/spec_helper.rb`). See
[../README.md](../README.md) for what to try and what the gaps are.

Most calls are written without parentheses: `@store.save order` in
`create_order`, `count.zero?` in `MemoryStore#empty?`, and `@store.empty?`
and `#{@store.count}` in `OrderHandler#summary`. Open **Calls** on
`summary` or **Trace to here** on `MemoryStore#count` to see them.

Regenerate coverage: `bundle install && bundle exec rspec` (writes
`coverage/lcov.info`; commit only that file from `coverage/`).
