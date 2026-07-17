## MODIFIED Requirements

### Requirement: Transport close robustness
The close() method SHALL handle concurrent access and process cleanup robustly.

#### Scenario: Close discards child only on successful reap
- **WHEN** close() is called and process exit times out after SIGKILL
- **THEN** the child SHALL remain in activeChildren for the atexit reaper

#### Scenario: Close acquires write lock with timeout
- **WHEN** close() is called while a write is blocked on a full stdin pipe
- **THEN** the lock acquire SHALL timeout after 5 seconds rather than blocking forever
