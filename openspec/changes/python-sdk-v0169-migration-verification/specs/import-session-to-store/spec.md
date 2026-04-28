## MODIFIED Requirements

### Requirement: Import local session transcript into store
`ImportSessionToStore` SHALL replay a local JSONL transcript into a `SessionStore`, including subagent transcripts.

#### Scenario: Import with subagent support
- **WHEN** `ImportSessionToStore(store, sessionID, dir, true)` is called
- **THEN** the system imports the main transcript and recursively imports all `<sessionId>/subagents/**/*.jsonl` files with appropriate subpath keys

#### Scenario: Import without subagent support
- **WHEN** `ImportSessionToStore(store, sessionID, dir, false)` is called
- **THEN** only the main transcript is imported

#### Scenario: Batching
- **WHEN** the transcript has more than 500 entries
- **THEN** the system calls `store.Append()` in batches of 500 entries

#### Scenario: .meta.json sidecar import
- **WHEN** a subagent directory contains a `.meta.json` file
- **THEN** the system reads it and appends it as an `agent_metadata` type entry to the subkey

#### Scenario: Project key from directory name
- **WHEN** the session file is found at `<projectDir>/<sessionID>.jsonl`
- **THEN** the project key is derived from `filepath.Base(projectDir)` (matching TranscriptMirrorBatcher's key derivation)
