# java-maven

A small orders service in Java with JUnit 5: `Orders` (validation and
pricing), `MemoryStore`, `Notifier` (a webhook that's hard to test) and
`OrderHandler` (request handling). See [../README.md](../README.md) for
what to try and what the gaps are. `Orders.grandTotal` passes `Orders::total`
as a method reference, which CodeDiver records as a `ref` edge.

Regenerate coverage: `mvn -q org.jacoco:jacoco-maven-plugin:prepare-agent test org.jacoco:jacoco-maven-plugin:report`
(writes `target/site/jacoco/jacoco.xml`; commit only that file from `target/`).
