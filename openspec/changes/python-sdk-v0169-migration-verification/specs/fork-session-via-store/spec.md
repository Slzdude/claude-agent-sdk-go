## MODIFIED Requirements

### Requirement: Fork session via store with upToMessageID
`ForkSessionViaStore` SHALL support forking from a specific message UUID and filtering sidechain entries.

#### Scenario: Fork with upToMessageID
- **WHEN** `ForkSessionViaStore(store, key, upToMessageID, title)` is called with a non-empty `upToMessageID`
- **THEN** only entries up to and including that message UUID are included in the fork

#### Scenario: Fork without upToMessageID
- **WHEN** `upToMessageID` is empty
- **THEN** the full transcript is forked

#### Scenario: Sidechain filtering
- **WHEN** entries have `isSidechain: true`
- **THEN** those entries are excluded from the fork

#### Scenario: Invalid upToMessageID
- **WHEN** `upToMessageID` is not a valid UUID
- **THEN** the system returns an error

#### Scenario: upToMessageID not found
- **WHEN** `upToMessageID` is not found in the transcript
- **THEN** the system returns an error

#### Scenario: Auto-derived title
- **WHEN** `title` is empty
- **THEN** the system derives a title from `customTitle`, `aiTitle`, or first prompt, appending " (fork)"
