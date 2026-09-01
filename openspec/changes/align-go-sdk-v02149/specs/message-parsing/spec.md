## MODIFIED Requirements

### Requirement: UserMessage parent fields
The system SHALL parse parent_tool_use_id and parent_agent_id from user messages.

#### Scenario: Subagent message with parent fields
- **WHEN** a user message has parent_tool_use_id="tu1" and parent_agent_id="agent-1"
- **THEN** the parsed UserMessage SHALL have both fields populated

#### Scenario: Top-level message without parent fields
- **WHEN** a user message has no parent_tool_use_id or parent_agent_id
- **THEN** both fields SHALL be empty strings
