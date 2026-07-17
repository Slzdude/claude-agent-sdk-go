## MODIFIED Requirements

### Requirement: stdout line parsing
The transport SHALL parse stdout lines correctly with proper whitespace handling.

#### Scenario: Strip whitespace from complete lines
- **WHEN** a complete line has leading/trailing whitespace (e.g. CRLF)
- **THEN** the whitespace SHALL be stripped before JSON parsing

#### Scenario: Skip non-JSON lines
- **WHEN** a line does not start with "{"
- **THEN** the line SHALL be skipped with a debug log, not treated as JSON

#### Scenario: JSON parse failure raises error
- **WHEN** a line starts with "{" but fails JSON parsing
- **THEN** a CLIJSONDecodeError SHALL be raised (not silently skipped)

#### Scenario: Tail partial line handling
- **WHEN** the stream ends with a partial line (no trailing newline)
- **THEN** the partial line SHALL be yielded if it parses as JSON, dropped otherwise
