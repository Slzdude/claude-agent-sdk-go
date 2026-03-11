//go:build e2e

package e2e_test

// test_agents_and_settings.go mirrors e2e-tests/test_agents_and_settings.py.

import (
	"testing"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

// TestAgentDefinition tests that custom agent definitions work in streaming mode.
// Mirrors test_agent_definition.
func TestAgentDefinition(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	opts := &claude.ClaudeAgentOptions{
		Agents: map[string]claude.AgentDefinition{
			"test-agent": {
				Description: "A test agent for verification",
				Prompt:      "You are a test agent. Always respond with 'Test agent activated'",
				Tools:       []string{"Read"},
				Model:       claude.AgentModelSonnet,
			},
		},
		MaxTurns: 1,
	}

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "What is 2 + 2?"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	foundInit := false
	for msg := range client.ReceiveResponse(ctx) {
		sys, ok := msg.(*claude.SystemMessage)
		if !ok || sys.Subtype != "init" {
			continue
		}
		foundInit = true
		agents, ok := sys.Data["agents"]
		if !ok {
			t.Fatalf("agents key missing from init message data: %v", sys.Data)
		}
		agentList, ok := agents.([]any)
		if !ok {
			t.Fatalf("agents should be []any, got %T: %v", agents, agents)
		}
		found := false
		for _, a := range agentList {
			if a.(string) == "test-agent" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("test-agent not found in agents: %v", agentList)
		}
		break
	}
	if !foundInit {
		t.Log("warning: did not receive init SystemMessage (may have been missed)")
	}
}

// TestAgentDefinitionWithQueryFunction tests that custom agent definitions work
// with the Query() function. Mirrors test_agent_definition_with_query_function.
func TestAgentDefinitionWithQueryFunction(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	opts := &claude.ClaudeAgentOptions{
		Agents: map[string]claude.AgentDefinition{
			"test-agent-query": {
				Description: "A test agent for query function verification",
				Prompt:      "You are a test agent.",
			},
		},
		MaxTurns: 1,
	}

	ch, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	msgs := collectMessages(ch)
	requireResult(t, msgs)
}
