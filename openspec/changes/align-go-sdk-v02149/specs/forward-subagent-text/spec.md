## ADDED Requirements

### Requirement: ForwardSubagentText option
The system SHALL support forwarding subagent text/thinking blocks in the message stream.

#### Scenario: Option set to true
- **WHEN** ForwardSubagentText is true
- **THEN** the CLI SHALL receive --forward-subagent-text flag
- **THEN** the initialize request SHALL include forwardSubagentText=true

#### Scenario: Option default
- **WHEN** ForwardSubagentText is false (default)
- **THEN** only tool_use/tool_result blocks from subagents SHALL be emitted
