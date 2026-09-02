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

// Skill name control character injection tests
// Matches Python's test_build_command_skills_rejects_control_characters

func TestValidateSkillName_ControlCharacters(t *testing.T) {
	controlChars := []string{
		"name\nwith\nnewlines",
		"name\twith\ttabs",
		"name\x00with\x00nulls",
		"name\x7fwith\x7fdel",
	}
	for _, name := range controlChars {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for control character in %q", name)
				}
			}()
			validateSkillName(name)
		})
	}
}

// Skill name C1 control character tests
// Matches Python's test_build_command_skills_rejects_c1_control_characters

func TestValidateSkillName_C1ControlCharacters(t *testing.T) {
	c1Chars := []string{
		"namewithnel",  // NEL (Next Line)
		"namewithcsi",  // CSI (Control Sequence Introducer)
	}
	for _, name := range c1Chars {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for C1 control character in %q", name)
				}
			}()
			validateSkillName(name)
		})
	}
}

// Skill name BOM character tests
// Matches Python's test_build_command_skills_rejects_byte_order_marks

func TestValidateSkillName_BOMCharacters(t *testing.T) {
	// BOM = U+FEFF, UTF-8 encoding: 0xEF 0xBB 0xBF
	bom := string([]byte{0xef, 0xbb, 0xbf})
	bomNames := []string{
		bom + "pdf",      // BOM at start
		"pdf" + bom,      // BOM at end
		"pd" + bom + "f", // BOM in middle
	}
	for _, name := range bomNames {
		t.Run("bom_in_name", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for BOM character in skill name")
				}
			}()
			validateSkillName(name)
		})
	}
}

// Skill name delimiter injection tests
// Matches Python's test_build_command_skills_rejects_rule_syntax_delimiters

func TestValidateSkillName_DelimiterInjection(t *testing.T) {
	// These names contain delimiters that could break --allowedTools parsing
	maliciousNames := []string{
		"x),Bash(*",           // Closes one rule, opens another
		"safe),Bash,Skill(dummy", // Closes and opens multiple rules
		"name,with,commas",    // Commas are rule separators
		"unbalanced(",         // Unbalanced open paren
		"unbalanced)",         // Unbalanced close paren
		"()",                  // Empty parens
	}
	for _, name := range maliciousNames {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for delimiter injection in %q", name)
				}
			}()
			validateSkillName(name)
		})
	}
}

// Skill name consecutive backslash tests
// Matches Python's test_build_command_skills_rejects_consecutive_backslashes

func TestValidateSkillName_ConsecutiveBackslashes(t *testing.T) {
	backslashNames := []string{
		"name\\\\with\\\\backslashes",   // Two consecutive backslashes
		"name\\\\\\\\with\\\\\\\\more", // Four consecutive backslashes
		"mid\\\\dle",                    // Backslashes in middle
	}
	for _, name := range backslashNames {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for consecutive backslashes in %q", name)
				}
			}()
			validateSkillName(name)
		})
	}
}

// Skill name trailing backslash tests
// Matches Python's test_build_command_skills_rejects_unpaired_trailing_backslash

func TestValidateSkillName_TrailingBackslash(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for trailing backslash")
		}
	}()
	validateSkillName("name\\")
}

// Skill name valid edge cases
// Ensures valid names are not rejected

func TestValidateSkillName_ValidEdgeCases(t *testing.T) {
	validNames := []string{
		"my-skill",
		"plugin:skill",
		"skill-name",
		"a",
		"skill123",
		"skill-name-with-dashes",
		"plugin:skill:name",
	}
	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			// Should not panic
			validateSkillName(name)
		})
	}
}
