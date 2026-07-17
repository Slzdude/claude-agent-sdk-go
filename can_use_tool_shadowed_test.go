package claude

import (
	"testing"
)

func TestWholeToolAllowed(t *testing.T) {
	tests := []struct {
		entry string
		want  string
	}{
		{"Read", "Read"},
		{"Read()", "Read"},
		{"Read(*)", "Read"},
		{"Bash(ls:*)", ""},
		{"Bash(ls)", ""},
		{"", ""},
		{"  ", ""},
		{"(bad)", ""},
		{"Read(abc", ""},
		{"Read(extra, args)", ""},
	}
	for _, tt := range tests {
		got := WholeToolAllowed(tt.entry)
		if got != tt.want {
			t.Errorf("WholeToolAllowed(%q) = %q, want %q", tt.entry, got, tt.want)
		}
	}
}

func TestGetCanUseToolShadowedWarning_BypassPermissions(t *testing.T) {
	msg := GetCanUseToolShadowedWarning(PermissionModeBypassPermissions, nil, nil)
	if msg == "" {
		t.Error("expected warning for bypassPermissions")
	}
}

func TestGetCanUseToolShadowedWarning_WholeTool(t *testing.T) {
	msg := GetCanUseToolShadowedWarning(PermissionModeDefault, []string{"Read", "Bash(ls:*)"}, nil)
	if msg == "" {
		t.Error("expected warning for whole-tool allowed_tools")
	}
	if !testContains(msg, "Read") {
		t.Errorf("warning should mention Read, got: %s", msg)
	}
}

func TestGetCanUseToolShadowedWarning_NarrowedOnly(t *testing.T) {
	msg := GetCanUseToolShadowedWarning(PermissionModeDefault, []string{"Bash(ls:*)"}, nil)
	if msg != "" {
		t.Errorf("expected no warning for narrowed tools, got: %s", msg)
	}
}

func TestGetCanUseToolShadowedWarning_SkillsAll(t *testing.T) {
	msg := GetCanUseToolShadowedWarning(PermissionModeDefault, nil, "all")
	if msg == "" {
		t.Error("expected warning for skills=all")
	}
	if !testContains(msg, "Skill") {
		t.Errorf("warning should mention Skill, got: %s", msg)
	}
}

func TestGetCanUseToolShadowedWarning_NoShadowing(t *testing.T) {
	msg := GetCanUseToolShadowedWarning(PermissionModeDefault, nil, nil)
	if msg != "" {
		t.Errorf("expected no warning, got: %s", msg)
	}
}

func testContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
