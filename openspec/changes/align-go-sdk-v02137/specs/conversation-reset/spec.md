## ADDED Requirements

### Requirement: ConversationResetMessage type
The system SHALL define ConversationResetMessage with new_conversation_id, uuid, session_id fields.

#### Scenario: conversation_reset message parsed
- **WHEN** a message with type="conversation_reset" is parsed
- **THEN** a ConversationResetMessage is returned with all required fields
