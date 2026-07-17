## ADDED Requirements

### Requirement: Warn when can_use_tool is shadowed
The system SHALL log a warning when `can_use_tool` is set alongside options that visibly shadow it.

#### Scenario: bypassPermissions shadows can_use_tool
- **WHEN** can_use_tool is set AND permission_mode is "bypassPermissions"
- **THEN** a warning SHALL be logged explaining bypassPermissions auto-approves every call

#### Scenario: allowed_tools whole-tool shadows can_use_tool
- **WHEN** can_use_tool is set AND allowed_tools contains "Read" (whole tool)
- **THEN** a warning SHALL be logged listing the shadowed tools

#### Scenario: allowed_tools with specifier does NOT shadow
- **WHEN** can_use_tool is set AND allowed_tools contains "Bash(ls:*)"
- **THEN** no warning SHALL be logged (specifier narrows the rule)

#### Scenario: skills="all" shadows can_use_tool
- **WHEN** can_use_tool is set AND skills is "all"
- **THEN** a warning SHALL be logged including "Skill" in shadowed list
