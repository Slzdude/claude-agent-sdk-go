## ADDED Requirements

### Requirement: resume_session_at option
The system SHALL support resume_session_at to branch from a specific transcript entry.

#### Scenario: resume_session_at set
- **WHEN** resume_session_at is set to a UUID
- **THEN** the CLI SHALL receive --resume-session-at=<uuid>

### Requirement: resume_drops_turn option
The system SHALL support resume_drops_turn for validated truncation.

#### Scenario: resume_drops_turn set
- **WHEN** resume_drops_turn is set to a UUID
- **THEN** the CLI SHALL receive --resume-drops-turn=<uuid>
