## ADDED Requirements

### Requirement: Remove unused internal functions
The system SHALL remove internal functions that are never called.

#### Scenario: parseMirrorErrorMessage removed
- **WHEN** the codebase is compiled
- **THEN** parseMirrorErrorMessage does not exist in message_parser.go

#### Scenario: appendIfMissing removed
- **WHEN** the codebase is compiled
- **THEN** appendIfMissing does not exist in transport.go
