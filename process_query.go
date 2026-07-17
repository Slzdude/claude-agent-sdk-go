package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/codes"
)

// processQuery is the core orchestrator: creates a transport, connects,
// initialises the session, optionally sends a user message, and returns
// a channel of typed Messages.
//
// Exactly one of prompt or promptCh must be set:
//   - prompt: one-shot single-message query
//   - promptCh: streaming multi-message query (used when CanUseTool is set)
//
// The returned channel is closed when the subprocess exits or ctx is cancelled.
func processQuery(
	ctx context.Context,
	prompt string,
	promptCh <-chan map[string]any,
	opts *ClaudeAgentOptions,
	_ any, // reserved for future use
) (<-chan Message, error) {
	// Validate mutual exclusions.
	if opts.CanUseTool != nil && opts.PermissionPromptToolName != "" {
		return nil, &CLIConnectionError{
			Message: "CanUseTool and PermissionPromptToolName are mutually exclusive",
		}
	}

	// Validate session store option combinations.
	if err := ValidateSessionStoreOptions(opts); err != nil {
		return nil, &CLIConnectionError{Message: err.Error()}
	}

	// Advisory: warn if can_use_tool is shadowed by allowed_tools or bypassPermissions.
	WarnIfCanUseToolShadowed(opts)

	// resume/continue + session_store: load the session from the store into a
	// temp CLAUDE_CONFIG_DIR for the subprocess to resume from.
	materialized, err := MaterializeResumeSession(ctx, opts)
	if err != nil {
		return nil, &CLIConnectionError{Message: err.Error()}
	}
	if materialized != nil {
		defer materialized.Cleanup()
	}

	configuredOpts := *opts
	if materialized != nil {
		configuredOpts = ApplyMaterializedOptions(configuredOpts, materialized)
	}
	if opts.CanUseTool != nil {
		configuredOpts.PermissionPromptToolName = "stdio"
	}

	t, err := newCLITransport(&configuredOpts)
	if err != nil {
		return nil, err
	}
	if err := t.connect(ctx); err != nil {
		return nil, err
	}

	// Collect SDK MCP servers.
	sdkServers := map[string]SdkMcpServer{}
	for name, cfg := range configuredOpts.MCPServers {
		if s, ok := cfg.(*MCPSdkServerConfig); ok && s.Instance != nil {
			sdkServers[name] = s.Instance
		}
	}

	// Collect agent definitions.
	var agentsMap map[string]map[string]any
	if len(configuredOpts.Agents) > 0 {
		agentsMap = make(map[string]map[string]any, len(configuredOpts.Agents))
		for name, def := range configuredOpts.Agents {
			b, _ := json.Marshal(def)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			agentsMap[name] = m
		}
	}

	// Initialize tracing BEFORE creating queryProto so hooks are registered
	// before Initialize() sends them to the CLI.
	var st *sessionTracer
	if configuredOpts.TracerProvider != nil {
		st = newSessionTracer(configuredOpts.TracerProvider)
		st.injectHooks(&configuredOpts)
	}

	q := newQueryProto(t, &configuredOpts)
	q.SetSDKMCPServers(sdkServers)
	q.SetAgents(agentsMap)
	if sp, ok := configuredOpts.SystemPrompt.(*SystemPromptPreset); ok && sp.ExcludeDynamicSections != nil {
		q.SetExcludeDynamicSections(sp.ExcludeDynamicSections)
	}
	if configuredOpts.Skills != nil {
		q.SetSkills(configuredOpts.Skills)
	}
	out := make(chan Message, 64)

	// Attach transcript mirror batcher if session store is configured.
	if configuredOpts.SessionStore != nil {
		projectsDir := getProjectsDir()
		if materialized != nil {
			projectsDir = filepath.Join(materialized.ConfigDir, "projects")
		}
		batcher := NewTranscriptMirrorBatcher(
			configuredOpts.SessionStore,
			projectsDir,
			func(key *SessionKey, errMsg string) {
				// Inject the mirror error as a synthesized system message so consumers
				// see it as a MirrorErrorMessage. Uses q.ReportMirrorError which writes
				// to q.rawOut; the conversion goroutine below parses and forwards it.
				q.ReportMirrorError(key, errMsg)
			},
			configuredOpts.SessionStoreFlush,
		)
		q.SetMirrorBatcher(batcher)
	}

	// Start the read loop BEFORE sending the initialize request.
	// This mirrors Python SDK's `await query.start()` then `await query.initialize()`.
	// Without this, Initialize() sends a control_request but no goroutine is reading
	// the response, causing a 30-second timeout.
	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		_ = t.close()
		return nil, err
	}

	// Send user input.
	if prompt != "" {
		if err := q.SendUserMessage(ctx, prompt); err != nil {
			_ = t.close()
			return nil, err
		}
	}
	// If a promptCh is provided the caller is responsible for sending messages.
	// We start a goroutine to relay them.
	if promptCh != nil {
		go func() {
			for {
				select {
				case raw, ok := <-promptCh:
					if !ok {
						_ = t.closeStdin()
						return
					}
					if err := q.SendRawMessage(ctx, raw); err != nil {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	} else {
		// For single-message queries: if hooks or SDK MCP servers are active,
		// defer stdin close until the first result is received (matching Python SDK).
		// This ensures the protocol stays open for inbound control requests.
		if len(sdkServers) > 0 || len(configuredOpts.Hooks) > 0 {
			go func() {
				q.WaitForFirstResult(ctx, 60*time.Second)
				_ = t.closeStdin()
			}()
		} else if prompt != "" {
			_ = t.closeStdin()
		}
	}

	if st != nil {
		// Traced path: create AGENT span and process messages with attribute extraction.
		ctx, rootSpan := st.startQuerySpan(ctx, "ClaudeAgentSDK.Query", prompt, configuredOpts.Model)

		go func() {
			defer close(out)
			defer func() { _ = t.close() }()
			defer rootSpan.End()
			defer st.endAll()

			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in traced message channel", "recover", r)
					rootSpan.RecordError(fmt.Errorf("panic in message iteration: %v", r))
					rootSpan.SetStatus(codes.Error, fmt.Sprintf("panic: %v", r))
				}
			}()

			outputMsgIndex := 0
			for raw := range rawCh {
				msg, err := parseMessage(raw)
				if err != nil || msg == nil {
					continue
				}
				st.processTracedMessage(ctx, rootSpan, msg, &outputMsgIndex)
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}()
	} else {
		// Non-traced path: simple passthrough.
		go func() {
			defer close(out)
			defer func() { _ = t.close() }()

			for raw := range rawCh {
				msg, err := parseMessage(raw)
				if err != nil || msg == nil {
					continue
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return out, nil
}
