## ADDED Requirements

### Requirement: TaskUpdatedStatus enum
The system SHALL define TaskUpdatedStatus with values "pending", "running", "paused", "completed", "failed", "killed".

#### Scenario: All values defined
- **WHEN** TaskUpdatedStatus constants are accessed
- **THEN** all 6 values are defined and match Python SDK

### Requirement: TerminalTaskStatuses set
The system SHALL define TerminalTaskStatuses containing "completed", "failed", "stopped", "killed".

#### Scenario: Terminal status check
- **WHEN** checking if a status is terminal
- **THEN** "completed", "failed", "stopped", "killed" return true; "pending", "running", "paused" return false

### Requirement: TaskUpdatedMessage type
The system SHALL define TaskUpdatedMessage with fields: TaskID, Patch, Status, SessionID, UUID.

#### Scenario: Message creation
- **WHEN** a TaskUpdatedMessage is created
- **THEN** all fields are accessible and correctly typed

### Requirement: task_updated parsing
The system SHALL parse system messages with subtype "task_updated" into TaskUpdatedMessage.

#### Scenario: Valid task_updated message
- **WHEN** a message with type="system", subtype="task_updated", task_id="t1", patch={"status":"completed"} is parsed
- **THEN** a TaskUpdatedMessage is returned with TaskID="t1", Status="completed"

#### Scenario: Missing patch field
- **WHEN** a task_updated message has no patch field
- **THEN** parsing succeeds with empty Patch map and Status=""

#### Scenario: Non-dict patch field
- **WHEN** a task_updated message has patch="invalid"
- **THEN** parsing succeeds with empty Patch map (defensive)

#### Scenario: Terminal status detection
- **WHEN** a TaskUpdatedMessage has Status="killed"
- **THEN** TerminalTaskStatuses["killed"] is true
