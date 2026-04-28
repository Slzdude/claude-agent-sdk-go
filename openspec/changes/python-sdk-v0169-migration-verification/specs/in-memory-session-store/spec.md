## MODIFIED Requirements

### Requirement: Real mtime in epoch milliseconds
`InMemorySessionStore` SHALL use `time.Now().UnixMilli()` for mtime instead of a hardcoded placeholder.

#### Scenario: ListSessions returns real mtime
- **WHEN** entries are appended to the store
- **THEN** `ListSessions()` returns `mtime` values that are real epoch-millisecond timestamps (> 1e12)

#### Scenario: ListSessionSummaries returns real mtime
- **WHEN** entries are appended to the store
- **THEN** `ListSessionSummaries()` returns summaries with `mtime` set to the storage write time

#### Scenario: Monotonically increasing
- **WHEN** multiple appends happen in sequence
- **THEN** each append's mtime is strictly greater than the previous
