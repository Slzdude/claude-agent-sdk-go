## ADDED Requirements

### Requirement: Task lifecycle tracking
The system SHALL track in-flight tasks and delay stdin close until all tasks complete.

#### Scenario: Result with in-flight tasks
- **WHEN** a result message arrives while tasks are in flight
- **THEN** stdin SHALL NOT be closed

#### Scenario: Result with no in-flight tasks
- **WHEN** a result message arrives with no tasks in flight
- **THEN** stdin SHALL be closed
