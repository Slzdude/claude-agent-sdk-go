## ADDED Requirements

### Requirement: ModelUsage struct
The system SHALL define ModelUsage with typed fields matching the CLI wire format (camelCase).

#### Scenario: ModelUsage fields
- **WHEN** a ModelUsage is created
- **THEN** it SHALL have inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens, webSearchRequests, costUSD, contextWindow, maxOutputTokens fields
