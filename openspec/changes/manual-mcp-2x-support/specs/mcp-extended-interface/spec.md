## ADDED Requirements

### Requirement: SdkMcpServer extended interface
The system SHALL extend SdkMcpServer with optional methods for resources and prompts.

#### Scenario: Server implements only tools
- **WHEN** a server only implements ListTools/CallTool
- **THEN** resources/list and prompts/list SHALL return empty results

#### Scenario: Server implements resources
- **WHEN** a server implements ListResources/ReadResource
- **THEN** resources/list and resources/read SHALL return correct results

### Requirement: New MCP types
The system SHALL define MCPResource, MCPResourceContent, MCPPrompt, MCPPromptResult types.

#### Scenario: MCPResource fields
- **WHEN** a MCPResource is created
- **THEN** it SHALL have URI, Name, Description, MimeType fields
