## MODIFIED Requirements

### Requirement: Defensive content block parsing
The message parser SHALL handle malformed content blocks gracefully.

#### Scenario: Non-dict content block in user message
- **WHEN** a user message contains a non-dict content block (e.g. string)
- **THEN** parsing SHALL return an error, not panic

#### Scenario: Non-list assistant content
- **WHEN** an assistant message has content as a string instead of a list
- **THEN** parsing SHALL return an error, not panic
