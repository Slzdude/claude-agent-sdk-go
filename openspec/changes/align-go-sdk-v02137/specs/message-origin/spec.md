## ADDED Requirements

### Requirement: MessageOrigin type
The system SHALL define MessageOriginKind with 9 values and MessageOrigin struct.

#### Scenario: All origin kinds defined
- **WHEN** MessageOriginKind constants are accessed
- **THEN** all 9 values (human, channel, peer, task-notification, coordinator, unclassified, observer, auto-continuation, observer-activity) are defined

### Requirement: Origin parsing
The system SHALL parse "origin" field from UserMessage and ResultMessage.

#### Scenario: Valid origin in message
- **WHEN** a message has origin={"kind":"task-notification","senderTaskId":"t1"}
- **THEN** the parsed message's Origin field SHALL contain the parsed MessageOrigin

#### Scenario: Missing origin
- **WHEN** a message has no origin field
- **THEN** the parsed message's Origin field SHALL be nil
