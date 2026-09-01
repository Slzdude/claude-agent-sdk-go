## ADDED Requirements

### Requirement: notifications/cancelled handler
The system SHALL handle notifications/cancelled requests and cancel the corresponding in-flight request.

#### Scenario: Cancel in-flight request
- **WHEN** a notifications/cancelled request arrives with a valid requestId
- **THEN** the corresponding request's context SHALL be cancelled
- **THEN** no response SHALL be sent (it's a notification)

#### Scenario: Cancel unknown request
- **WHEN** a notifications/cancelled request arrives with an unknown requestId
- **THEN** the notification SHALL be silently ignored
