package claude

import (
	"testing"
)

func TestRejectNonListSkills_Valid(t *testing.T) {
	// Should not panic for valid inputs
	rejectNonListSkills(nil)
	rejectNonListSkills([]string{"skill-a", "skill-b"})
	rejectNonListSkills("all")
}

func TestRejectNonListSkills_InvalidString(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for string skills")
		}
	}()
	rejectNonListSkills("my-skill")
}

func TestValidateSkillName_Valid(t *testing.T) {
	// Should not panic for valid names
	validateSkillName("my-skill")
	validateSkillName("plugin:skill")
	validateSkillName("skill-name")
}

func TestValidateSkillName_Empty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	validateSkillName("")
}

func TestValidateSkillName_Whitespace(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for whitespace name")
		}
	}()
	validateSkillName("  my-skill  ")
}

func TestValidateSkillName_Parentheses(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for parentheses")
		}
	}()
	validateSkillName("my(skill)")
}

func TestValidateSkillName_Star(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for *")
		}
	}()
	validateSkillName("*")
}

func TestValidateSkillName_StartsWithSlash(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for / prefix")
		}
	}()
	validateSkillName("/my-skill")
}

func TestValidateSkillName_WildcardSuffix(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for wildcard suffix")
		}
	}()
	validateSkillName("my-skill:*")
}

func TestRejectWindowsCmdMetacharacters_NonWindows(t *testing.T) {
	// On non-Windows, this should be a no-op
	rejectWindowsCmdMetacharacters("resume", "test&value")
}
