# ruby-rspec

A small orders service in Ruby with RSpec: `Orders` (validation and
pricing), `Orders::MemoryStore`, `Orders::Notifier` (a webhook that's hard
to test) and `Orders::OrderHandler` (request handling). SimpleCov writes
LCOV through `simplecov-lcov` (configured in `spec/spec_helper.rb`). See
[../README.md](../README.md) for what to try and what the gaps are.

Regenerate coverage: `bundle install && bundle exec rspec` (writes
`coverage/lcov.info`; commit only that file from `coverage/`).
