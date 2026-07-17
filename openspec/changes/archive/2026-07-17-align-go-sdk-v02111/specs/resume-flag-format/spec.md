## MODIFIED Requirements

### Requirement: CLI flag injection prevention
The transport SHALL pass --resume and --session-id as single argv tokens to prevent flag injection.

#### Scenario: Normal resume value
- **WHEN** resume is "abc123"
- **THEN** the CLI args SHALL contain "--resume=abc123" (not "--resume" "abc123")

#### Scenario: Dash-leading resume value
- **WHEN** resume is "--evil"
- **THEN** the CLI args SHALL contain "--resume=--evil" (not "--resume" "--evil")
