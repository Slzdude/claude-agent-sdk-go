## ADDED Requirements

### Requirement: ResultError type
The system SHALL define ResultError embedding ProcessError with additional fields for structured error information.

#### Scenario: ResultError fields
- **WHEN** a ResultError is created
- **THEN** it SHALL have Subtype, Errors, Result, APIErrorStatus, TerminalReason, SessionID, Data fields

#### Scenario: ResultError is a ProcessError
- **WHEN** code checks `errors.As(err, &processError)`
- **THEN** a ResultError SHALL match

### Requirement: _error_result_text helper
The system SHALL extract error text from result messages with fallback chain: errors[] → result → subtype → api_error_status.

#### Scenario: Error with errors array
- **WHEN** result has errors=["tool timed out", "ENOENT"]
- **THEN** text SHALL be "tool timed out; ENOENT"

#### Scenario: Error with result text
- **WHEN** result has is_error=true, subtype="success", result="API Error: 429"
- **THEN** text SHALL be "API Error: 429"

### Requirement: ResultError integration
The system SHALL replace ProcessError with ResultError when an error result was received before subprocess exit.

#### Scenario: ProcessError after error result
- **WHEN** CLI emits error result then exits non-zero
- **THEN** the error SHALL be a ResultError with the result's structured data
