## ADDED Requirements

### Requirement: 14-contract conformance test suite
The system SHALL provide a `RunSessionStoreConformance(t, makeStore)` function that tests 14 behavioral contracts against any `SessionStore` implementation.

#### Scenario: InMemorySessionStore passes all contracts
- **WHEN** `RunSessionStoreConformance` is called with `NewInMemorySessionStore`
- **THEN** all 14 subtests pass

### Requirement: Required contracts (append + load)
The system SHALL test the following required contracts:

#### Scenario: Append then load returns same entries in order
- **WHEN** entries are appended to a key
- **THEN** `Load()` returns the same entries in the same order

#### Scenario: Load unknown key returns nil
- **WHEN** `Load()` is called for a key that was never written
- **THEN** it returns nil

#### Scenario: Multiple appends preserve call order
- **WHEN** multiple `Append()` calls are made to the same key
- **THEN** `Load()` returns all entries in call order

#### Scenario: Empty append is no-op
- **WHEN** `Append()` is called with an empty slice
- **THEN** the store state is unchanged

#### Scenario: Subpath keys stored independently
- **WHEN** entries are appended to a key with and without subpath
- **THEN** each key's entries are independent

#### Scenario: Project key isolation
- **WHEN** entries are appended under different project keys
- **THEN** each project's entries are isolated

### Requirement: Optional contracts (list/delete/subkeys)
The system SHALL test optional methods when the store implements them.

#### Scenario: ListSessions returns correct sessions
- **WHEN** sessions are appended under a project key
- **THEN** `ListSessions()` returns the correct session IDs with mtime > 1e12 (epoch-ms)

#### Scenario: Delete cascades to subkeys
- **WHEN** a main key is deleted
- **THEN** all subkeys under that session are also deleted

#### Scenario: ListSubkeys returns subpaths
- **WHEN** entries are appended with subpaths
- **THEN** `ListSubkeys()` returns the correct subpath strings
