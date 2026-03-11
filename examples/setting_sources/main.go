// setting_sources demonstrates controlling which settings files Claude loads.
//
// Setting sources determine where Claude Code loads configurations from:
//   - "user":    Global user settings (~/.claude/)
//   - "project": Project-level settings (.claude/ in project)
//   - "local":   Local gitignored settings (.claude-local/)
//
// Usage:
//
//	go run . default
//	go run . user_only
//	go run . project_and_user
//	go run . all
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

// sdkDir returns the repo root (two levels up from examples/setting_sources/).
func sdkDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func extractSlashCommands(msg *claude.SystemMessage) []any {
	if msg.Subtype == "init" {
		if cmds, ok := msg.Data["slash_commands"]; ok {
			if list, ok := cmds.([]any); ok {
				return list
			}
		}
	}
	return nil
}

func exampleDefault(ctx context.Context) {
	fmt.Println("=== Default Behavior Example ===")
	fmt.Println("Setting sources: nil (default)")
	fmt.Println("Expected: No custom slash commands will be available")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{CWD: sdkDir()}
	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}
	for msg := range msgs {
		if sm, ok := msg.(*claude.SystemMessage); ok {
			cmds := extractSlashCommands(sm)
			fmt.Printf("Available slash commands: %v\n", cmds)
			hasCommit := false
			for _, c := range cmds {
				if c == "commit" {
					hasCommit = true
				}
			}
			if hasCommit {
				fmt.Println("❌ /commit is available (unexpected)")
			} else {
				fmt.Println("✓ /commit is NOT available (expected - no settings loaded)")
			}
			break
		}
	}
	fmt.Println()
}

func exampleUserOnly(ctx context.Context) {
	fmt.Println("=== User Settings Only Example ===")
	fmt.Println("Setting sources: [\"user\"]")
	fmt.Println("Expected: Project slash commands (like /commit) will NOT be available")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{
		SettingSources: []claude.SettingSource{claude.SettingSourceUser},
		CWD:            sdkDir(),
	}
	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}
	for msg := range msgs {
		if sm, ok := msg.(*claude.SystemMessage); ok {
			cmds := extractSlashCommands(sm)
			fmt.Printf("Available slash commands: %v\n", cmds)
			hasCommit := false
			for _, c := range cmds {
				if c == "commit" {
					hasCommit = true
				}
			}
			if hasCommit {
				fmt.Println("❌ /commit is available (unexpected)")
			} else {
				fmt.Println("✓ /commit is NOT available (expected)")
			}
			break
		}
	}
	fmt.Println()
}

func exampleProjectAndUser(ctx context.Context) {
	fmt.Println("=== Project + User Settings Example ===")
	fmt.Println("Setting sources: [\"user\", \"project\"]")
	fmt.Println("Expected: Project slash commands (like /commit) WILL be available")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{
		SettingSources: []claude.SettingSource{claude.SettingSourceUser, claude.SettingSourceProject},
		CWD:            sdkDir(),
	}
	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}
	for msg := range msgs {
		if sm, ok := msg.(*claude.SystemMessage); ok {
			cmds := extractSlashCommands(sm)
			fmt.Printf("Available slash commands: %v\n", cmds)
			hasCommit := false
			for _, c := range cmds {
				if c == "commit" {
					hasCommit = true
				}
			}
			if hasCommit {
				fmt.Println("✓ /commit is available (expected)")
			} else {
				fmt.Println("❌ /commit is NOT available (unexpected)")
			}
			break
		}
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	examples := map[string]func(context.Context){
		"default":          exampleDefault,
		"user_only":        exampleUserOnly,
		"project_and_user": exampleProjectAndUser,
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <example_name>")
		fmt.Println("\nAvailable examples:")
		fmt.Println("  all - Run all examples")
		for name := range examples {
			fmt.Println(" ", name)
		}
		os.Exit(0)
	}

	name := os.Args[1]
	fmt.Println("Starting Claude SDK Setting Sources Examples...")
	fmt.Println("==================================================")
	fmt.Println()

	if name == "all" {
		for _, fn := range examples {
			fn(ctx)
			fmt.Println("--------------------------------------------------")
			fmt.Println()
		}
		return
	}

	fn, ok := examples[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: Unknown example %q\n", name)
		os.Exit(1)
	}
	fn(ctx)
}
