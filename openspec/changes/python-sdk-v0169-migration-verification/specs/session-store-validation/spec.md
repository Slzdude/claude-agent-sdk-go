## ADDED Requirements

### Requirement: Validate incompatible option combinations
The system SHALL check for invalid `ClaudeAgentOptions` combinations involving `SessionStore` before subprocess spawn.

#### Scenario: continue_conversation without list_sessions
- **WHEN** `ContinueConversation` is true, `Resume` is empty, and `SessionStore` is set
- **THEN** the system returns an error requiring the store to implement `ListSessions()`

#### Scenario: session_store with enable_file_checkpointing
- **WHEN** both `SessionStore` and `EnableFileCheckpointing` are set
- **THEN** the system returns an error because they are incompatible

#### Scenario: Valid combinations pass
- **WHEN** `SessionStore` is set with `Resume` (not `ContinueConversation`)
- **THEN** validation passes without error

#### Scenario: No store skips validation
- **WHEN** `SessionStore` is nil
- **THEN** validation passes without error
