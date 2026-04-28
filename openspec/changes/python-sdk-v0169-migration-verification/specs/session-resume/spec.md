## ADDED Requirements

### Requirement: Materialize session from store to temp directory
The system SHALL load a session from `SessionStore` and write it to a temporary `CLAUDE_CONFIG_DIR` so the CLI subprocess can resume from it.

#### Scenario: Resume with explicit session ID
- **WHEN** `opts.Resume` is set and `opts.SessionStore` is non-nil
- **THEN** the system calls `store.Load()` with the session key, writes entries to `<tmpDir>/projects/<projectKey>/<sessionID>.jsonl`, and returns a `MaterializedResume` with the temp dir path and session ID

#### Scenario: Continue conversation (most recent session)
- **WHEN** `opts.ContinueConversation` is true and `opts.SessionStore` is non-nil
- **THEN** the system calls `store.ListSessions()`, sorts by mtime descending, skips sidechain sessions, loads the first non-empty session, and returns it as the resume candidate

#### Scenario: No store or no resume flag
- **WHEN** `opts.SessionStore` is nil, or both `opts.Resume` and `opts.ContinueConversation` are unset
- **THEN** the system returns nil and the caller falls through to normal resume/spawn

#### Scenario: Store returns empty entries
- **WHEN** `store.Load()` returns empty entries for the requested session
- **THEN** the system returns nil (no materialization needed)

### Requirement: Copy auth files with refreshToken redaction
The system SHALL copy `.credentials.json` (with `claudeAiOauth.refreshToken` removed) and `.claude.json` to the temp directory.

#### Scenario: Credentials file exists
- **WHEN** `.credentials.json` exists in the source config dir
- **THEN** the system writes a copy to `<tmpDir>/.credentials.json` with `refreshToken` removed from `claudeAiOauth`

#### Scenario: Credentials file missing
- **WHEN** `.credentials.json` does not exist
- **THEN** the system skips silently (API key auth, etc.)

### Requirement: Materialize subagent transcripts
The system SHALL load subagent transcripts via `store.ListSubkeys()` and write them to the temp directory.

#### Scenario: Subkeys exist
- **WHEN** `store.ListSubkeys()` returns subpaths like `subagents/agent-1`
- **THEN** the system loads each subkey's entries, partitions transcript vs metadata, writes `<subpath>.jsonl` and `<subpath>.meta.json`

#### Scenario: Unsafe subpath rejected
- **WHEN** a subpath is empty, absolute, contains `..`, or escapes the session directory
- **THEN** the system skips that subpath with a warning and continues

### Requirement: Apply materialized options
The system SHALL return a copy of options with `CLAUDE_CONFIG_DIR` set to the temp dir, `Resume` set to the resolved session ID, and `ContinueConversation` cleared.

#### Scenario: Options repointed
- **WHEN** `ApplyMaterializedOptions(opts, materialized)` is called
- **THEN** the returned options have `Env["CLAUDE_CONFIG_DIR"]` = temp dir path, `Resume` = session ID, `ContinueConversation` = false

### Requirement: Cleanup temp directory
The system SHALL remove the temp directory after the subprocess exits, with retry on transient errors.

#### Scenario: Normal cleanup
- **WHEN** the subprocess exits
- **THEN** `materialized.Cleanup()` removes the temp directory

#### Scenario: Cleanup with retry
- **WHEN** `os.RemoveAll` fails with EBUSY/EPERM/etc.
- **THEN** the system retries up to 4 times with 100ms backoff, then falls back to ignore-errors
