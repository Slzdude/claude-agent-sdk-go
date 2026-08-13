## ADDED Requirements

### Requirement: Windows command-line safety
The system SHALL reject resume/session_id values containing Windows cmd.exe metacharacters on Windows.

#### Scenario: Windows metacharacter in resume
- **WHEN** resume contains `&` or `|` on Windows
- **THEN** a ValueError SHALL be raised
