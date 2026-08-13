## ADDED Requirements

### Requirement: Skills parameter validation
The system SHALL validate that skills is a list or "all", and reject invalid skill names.

#### Scenario: skills is a string (not "all")
- **WHEN** skills is set to "my-skill" (string, not list)
- **THEN** a TypeError SHALL be raised

#### Scenario: skill name with invalid chars
- **WHEN** a skill name contains parentheses or commas
- **THEN** a ValueError SHALL be raised
