## MODIFIED Requirements

### Requirement: Incremental session summary derivation
`FoldSessionSummary` SHALL incrementally derive session metadata from entries with set-once fields, first-prompt extraction, and last-wins field mapping.

#### Scenario: Set-once fields
- **WHEN** entries contain `isSidechain`, `timestamp`, and `cwd` fields
- **THEN** `is_sidechain`, `created_at` (epoch ms), and `cwd` are set from the first entry only and not overwritten by subsequent entries

#### Scenario: First-prompt extraction with filtering
- **WHEN** entries contain user messages
- **THEN** the system skips `isMeta`, `isCompactSummary`, `tool_result` messages, slash-commands (`<command-name>`), and auto-generated patterns (`<local-command-stdout>`, `<session-start-hook>`, etc.), and sets `first_prompt` from the first real user text (truncated to 200 runes)

#### Scenario: Slash-command fallback
- **WHEN** the first user message is a slash-command
- **THEN** the system stashes it as `command_fallback` and continues looking for a real prompt

#### Scenario: First-prompt locked
- **WHEN** `first_prompt` has been set
- **THEN** subsequent entries do not overwrite it

#### Scenario: Last-wins field mapping
- **WHEN** entries contain `customTitle`, `aiTitle`, `summary`, `lastPrompt`, `gitBranch`
- **THEN** they are mapped to `custom_title`, `ai_title`, `summary_hint`, `last_prompt`, `git_branch` respectively (last-wins)

#### Scenario: Tag handling
- **WHEN** a `tag` entry has a non-empty tag value
- **THEN** `data["tag"]` is set; when the tag is empty, the key is deleted

#### Scenario: mtime not set by fold
- **WHEN** `FoldSessionSummary` is called
- **THEN** the returned summary's `Mtime` is unchanged from `prev` (or 0 for new); the adapter must stamp it after persisting
