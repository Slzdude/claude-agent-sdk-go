package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultMaxBufferSize     = 1024 * 1024
	minimumClaudeCodeVersion = "2.0.0"
	sdkVersion               = "0.2.121"
)

// Track live CLI subprocesses so we can terminate them when the parent process
// exits. This mirrors the TypeScript SDK's parent-exit cleanup and prevents
// orphaned claude processes from leaking when callers crash or exit before
// calling Close().
var (
	activeChildrenMu sync.Mutex
	activeChildren   = make(map[*exec.Cmd]struct{})
)

func registerChild(cmd *exec.Cmd) {
	activeChildrenMu.Lock()
	activeChildren[cmd] = struct{}{}
	activeChildrenMu.Unlock()
}

func unregisterChild(cmd *exec.Cmd) {
	activeChildrenMu.Lock()
	delete(activeChildren, cmd)
	activeChildrenMu.Unlock()
}

func killActiveChildren() {
	activeChildrenMu.Lock()
	cmds := make([]*exec.Cmd, 0, len(activeChildren))
	for cmd := range activeChildren {
		cmds = append(cmds, cmd)
	}
	activeChildrenMu.Unlock()

	for _, cmd := range cmds {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}
}

func init() {
	// Register signal handler to kill active children on parent exit.
	// This prevents orphaned claude processes when the parent crashes or exits
	// without calling Close().
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		killActiveChildren()
		os.Exit(1)
	}()
}

// transport manages the raw subprocess I/O.
type cliTransport struct {
	opts          *ClaudeAgentOptions
	cliPath       string
	cwd           string
	maxBufferSize int

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner

	mu      sync.Mutex
	err     error
	closed  bool
	stdinMu sync.Mutex
	// writeFailed is set to true on the first write error (broken pipe, etc.).
	// Subsequent writes fail fast without touching the pipe. Mirrors Python's
	// _ready = False / _exit_error pattern.
	writeFailed  bool
	writeFailErr error
}

func newCLITransport(opts *ClaudeAgentOptions) (*cliTransport, error) {
	t := &cliTransport{opts: opts}

	cliPath := opts.CLIPath
	if cliPath == "" {
		found, err := t.findCLI()
		if err != nil {
			return nil, err
		}
		cliPath = found
	}
	t.cliPath = cliPath
	t.cwd = opts.CWD
	t.maxBufferSize = opts.MaxBufferSize
	if t.maxBufferSize <= 0 {
		t.maxBufferSize = defaultMaxBufferSize
	}
	return t, nil
}

func (t *cliTransport) findCLI() (string, error) {
	if p := t.findBundledCLI(); p != "" {
		return p, nil
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".npm-global", "bin", "claude"),
		"/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, "node_modules", ".bin", "claude"),
		filepath.Join(home, ".yarn", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", &CLINotFoundError{}
}

func (t *cliTransport) findBundledCLI() string {
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	_, srcFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	bundled := filepath.Join(filepath.Dir(srcFile), "_bundled", name)
	if fi, err := os.Stat(bundled); err == nil && !fi.IsDir() {
		return bundled
	}
	return ""
}

func (t *cliTransport) buildCommand() []string {
	opts := t.opts
	cmd := []string{t.cliPath, "--output-format", "stream-json", "--verbose"}

	switch sp := opts.SystemPrompt.(type) {
	case nil:
		// Don't pass --system-prompt when unset — let the CLI use its default
		// system prompt which includes skill listings, memory, and other context.
		// Passing --system-prompt "" overrides the default and strips skills.
	case string:
		cmd = append(cmd, "--system-prompt", sp)
	case *SystemPromptPreset:
		if sp.Append != "" {
			cmd = append(cmd, "--append-system-prompt", sp.Append)
		}
	case *SystemPromptFile:
		cmd = append(cmd, "--system-prompt-file", sp.Path)
	}

	switch tv := opts.Tools.(type) {
	case []string:
		if len(tv) == 0 {
			cmd = append(cmd, "--tools", "")
		} else {
			cmd = append(cmd, "--tools", strings.Join(tv, ","))
		}
	case *ToolsPreset:
		cmd = append(cmd, "--tools", "default")
	}

	// Apply skills defaults: inject Skill tool and default setting_sources.
	effectiveAllowedTools := append([]string{}, opts.AllowedTools...)
	effectiveSettingSources := opts.SettingSources
	if opts.Skills != nil {
		switch s := opts.Skills.(type) {
		case string:
			if s == "all" {
				if !contains(effectiveAllowedTools, "Skill") {
					effectiveAllowedTools = append(effectiveAllowedTools, "Skill")
				}
			}
		case []string:
			for _, name := range s {
				pattern := "Skill(" + name + ")"
				if !contains(effectiveAllowedTools, pattern) {
					effectiveAllowedTools = append(effectiveAllowedTools, pattern)
				}
			}
		}
		if len(effectiveSettingSources) == 0 {
			effectiveSettingSources = []SettingSource{SettingSourceUser, SettingSourceProject}
		}
	}

	if len(effectiveAllowedTools) > 0 {
		cmd = append(cmd, "--allowedTools", strings.Join(effectiveAllowedTools, ","))
	}
	if opts.MaxTurns > 0 {
		cmd = append(cmd, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}
	if opts.MaxBudgetUSD != nil {
		cmd = append(cmd, "--max-budget-usd", strconv.FormatFloat(*opts.MaxBudgetUSD, 'f', -1, 64))
	}
	if len(opts.DisallowedTools) > 0 {
		cmd = append(cmd, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}
	if opts.TaskBudget != nil {
		cmd = append(cmd, "--task-budget", strconv.Itoa(opts.TaskBudget.Total))
	}
	if opts.Model != "" {
		cmd = append(cmd, "--model", opts.Model)
	}
	if opts.FallbackModel != "" {
		cmd = append(cmd, "--fallback-model", opts.FallbackModel)
	}
	if len(opts.Betas) > 0 {
		ss := make([]string, len(opts.Betas))
		for i, b := range opts.Betas {
			ss[i] = string(b)
		}
		cmd = append(cmd, "--betas", strings.Join(ss, ","))
	}
	if opts.PermissionPromptToolName != "" {
		cmd = append(cmd, "--permission-prompt-tool", opts.PermissionPromptToolName)
	}
	if opts.PermissionMode != "" {
		cmd = append(cmd, "--permission-mode", string(opts.PermissionMode))
	}
	if opts.ContinueConversation {
		cmd = append(cmd, "--continue")
	}
	if opts.Resume != "" {
		cmd = append(cmd, "--resume="+opts.Resume)
	}
	if opts.SessionID != "" {
		cmd = append(cmd, "--session-id="+opts.SessionID)
	}
	if opts.ForkSession {
		cmd = append(cmd, "--fork-session")
	}
	if sv := t.buildSettingsValue(); sv != "" {
		cmd = append(cmd, "--settings", sv)
	}
	for _, d := range opts.AddDirs {
		cmd = append(cmd, "--add-dir", d)
	}
	if opts.MCPConfigPath != "" {
		cmd = append(cmd, "--mcp-config", opts.MCPConfigPath)
	} else if len(opts.MCPServers) > 0 {
		mcpMap := make(map[string]any, len(opts.MCPServers))
		for name, cfg := range opts.MCPServers {
			switch c := cfg.(type) {
			case *MCPSdkServerConfig:
				mcpMap[name] = map[string]any{"type": "sdk", "name": c.Name}
			case *MCPStdioServerConfig:
				mcpMap[name] = map[string]any{"type": "stdio", "command": c.Command, "args": c.Args, "env": c.Env}
			case *MCPSSEServerConfig:
				mcpMap[name] = map[string]any{"type": "sse", "url": c.URL, "headers": c.Headers}
			case *MCPHTTPServerConfig:
				mcpMap[name] = map[string]any{"type": "http", "url": c.URL, "headers": c.Headers}
			}
		}
		if b, err := json.Marshal(map[string]any{"mcpServers": mcpMap}); err == nil {
			cmd = append(cmd, "--mcp-config", string(b))
		}
	}
	if opts.IncludePartialMessages {
		cmd = append(cmd, "--include-partial-messages")
	}
	if opts.SessionStore != nil {
		cmd = append(cmd, "--session-mirror")
	}
	if opts.IncludeHookEvents {
		cmd = append(cmd, "--include-hook-events")
	}
	if opts.StrictMCPConfig {
		cmd = append(cmd, "--strict-mcp-config")
	}
	// --setting-sources: emit if non-nil (even empty slice disables all sources).
	// Matches Python SDK v0.1.53+ behaviour: `if effective_setting_sources is not None`.
	if effectiveSettingSources != nil {
		sourceParts := make([]string, len(effectiveSettingSources))
		for i, s := range effectiveSettingSources {
			sourceParts[i] = string(s)
		}
		cmd = append(cmd, "--setting-sources="+strings.Join(sourceParts, ","))
	}
	for _, p := range opts.Plugins {
		if p.Type == "local" {
			cmd = append(cmd, "--plugin-dir", p.Path)
		}
	}
	for flag, val := range opts.ExtraArgs {
		if val == nil {
			cmd = append(cmd, "--"+flag)
		} else {
			cmd = append(cmd, "--"+flag, *val)
		}
	}

	// Thinking — use --thinking flag for adaptive/disabled, --max-thinking-tokens for enabled.
	if opts.Thinking != nil {
		switch t := opts.Thinking.(type) {
		case *ThinkingAdaptive:
			cmd = append(cmd, "--thinking", "adaptive")
			if t.Display != "" {
				cmd = append(cmd, "--thinking-display", string(t.Display))
			}
		case *ThinkingEnabled:
			cmd = append(cmd, "--max-thinking-tokens", strconv.Itoa(t.BudgetTokens))
			if t.Display != "" {
				cmd = append(cmd, "--thinking-display", string(t.Display))
			}
		case *ThinkingDisabled:
			cmd = append(cmd, "--thinking", "disabled")
		}
	} else if opts.MaxThinkingTokens != nil {
		cmd = append(cmd, "--max-thinking-tokens", strconv.Itoa(*opts.MaxThinkingTokens))
	}
	if opts.Effort != "" {
		cmd = append(cmd, "--effort", string(opts.Effort))
	}
	if len(opts.OutputFormat) > 0 {
		if fmtType, ok := opts.OutputFormat["type"].(string); ok && fmtType == "json_schema" {
			if schema, ok := opts.OutputFormat["schema"]; ok {
				if b, err := json.Marshal(schema); err == nil {
					cmd = append(cmd, "--json-schema", string(b))
				}
			}
		}
	}

	cmd = append(cmd, "--input-format", "stream-json")
	return cmd
}

func (t *cliTransport) buildSettingsValue() string {
	opts := t.opts
	hasSandbox := opts.Sandbox != nil
	if opts.Settings == "" && !hasSandbox {
		return ""
	}
	if opts.Settings != "" && !hasSandbox {
		return opts.Settings
	}
	obj := map[string]any{}
	if opts.Settings != "" {
		s := strings.TrimSpace(opts.Settings)
		if strings.HasPrefix(s, "{") {
			_ = json.Unmarshal([]byte(s), &obj)
		} else if data, err := os.ReadFile(s); err == nil {
			_ = json.Unmarshal(data, &obj)
		}
	}
	if hasSandbox {
		obj["sandbox"] = opts.Sandbox
	}
	b, _ := json.Marshal(obj)
	return string(b)
}

func (t *cliTransport) connect(ctx context.Context) error {
	if os.Getenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK") == "" {
		t.checkVersion()
	}

	args := t.buildCommand()
	t.cmd = exec.CommandContext(ctx, args[0], args[1:]...)

	// Build environment: inherited (filtered) + user overrides + SDK defaults.
	// Filter CLAUDECODE so SDK subprocesses don't think they're inside Claude Code.
	// CLAUDE_CODE_ENTRYPOINT is set AFTER user env so SDK value always wins,
	// matching Python SDK behavior.
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		env = append(env, e)
	}
	for k, v := range t.opts.Env {
		env = append(env, k+"="+v)
	}
	// SDK-controlled vars — always set, cannot be overridden by user env.
	env = append(env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
	env = append(env, "CLAUDE_AGENT_SDK_VERSION="+sdkVersion)
	if t.opts.EnableFileCheckpointing {
		env = append(env, "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
	}
	// Propagate W3C trace context (TRACEPARENT/TRACESTATE).
	// Uses OTEL SDK if available, otherwise forwards from process env.
	// Matches Python SDK's opentelemetry.propagate.inject() behavior.
	otelEnv := make(map[string]string)
	for k, v := range t.opts.Env {
		otelEnv[k] = v
	}
	injectTraceContext(otelEnv)
	for _, key := range []string{"TRACEPARENT", "TRACESTATE"} {
		if v, ok := otelEnv[key]; ok {
			env = append(env, key+"="+v)
		}
	}
	if t.cwd != "" {
		env = append(env, "PWD="+t.cwd)
	}
	t.cmd.Env = env
	if t.cwd != "" {
		t.cmd.Dir = t.cwd
	}

	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return &CLIConnectionError{Message: "failed to create stdin pipe", Cause: err}
	}
	stdoutPipe, err := t.cmd.StdoutPipe()
	if err != nil {
		return &CLIConnectionError{Message: "failed to create stdout pipe", Cause: err}
	}

	// Pipe stderr only when the caller registered a callback.
	var stderrPipe io.ReadCloser
	if t.opts.Stderr != nil {
		stderrPipe, err = t.cmd.StderrPipe()
		if err != nil {
			return &CLIConnectionError{Message: "failed to create stderr pipe", Cause: err}
		}
	}

	if t.opts.User != "" {
		if err := setCmdUser(t.cmd, t.opts.User); err != nil {
			return &CLIConnectionError{Message: fmt.Sprintf("failed to set subprocess user %q", t.opts.User), Cause: err}
		}
	}

	if err := t.cmd.Start(); err != nil {
		if t.cwd != "" {
			if _, statErr := os.Stat(t.cwd); os.IsNotExist(statErr) {
				return &CLIConnectionError{Message: fmt.Sprintf("working directory does not exist: %s", t.cwd)}
			}
		}
		return &CLINotFoundError{CLIPath: t.cliPath, Cause: err}
	}
	registerChild(t.cmd)

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, t.maxBufferSize), t.maxBufferSize)
	t.stdout = scanner

	if stderrPipe != nil {
		go t.drainStderr(stderrPipe)
	}
	return nil
}

func (t *cliTransport) drainStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" && t.opts.Stderr != nil {
			// Isolate per-line so a panic in the user's callback doesn't
			// terminate the loop and silently drop every subsequent line
			// for the rest of the session. Matches Python SDK's try/except.
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[transport] stderr callback panicked: %v", r)
					}
				}()
				t.opts.Stderr(line)
			}()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[transport] stderr read error: %v", err)
	}
}

// readMessages delivers raw JSON objects from stdout into a channel.
// parseStdoutLine parses one complete line of the CLI's NDJSON stdout.
// Returns (msg, nil) for valid JSON lines, (nil, nil) for blank/non-JSON lines,
// and (nil, err) for lines that look like JSON but fail to parse.
// Matches Python SDK's _parse_stdout_line.
func parseStdoutLine(line string) (map[string]any, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	if line[0] != '{' {
		log.Printf("Skipping non-JSON line from CLI stdout: %s", line[:min(len(line), 200)])
		return nil, nil
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, &CLIJSONDecodeError{Line: line, Cause: err}
	}
	return msg, nil
}

func (t *cliTransport) readMessages(ctx context.Context) <-chan map[string]any {
	ch := make(chan map[string]any, 256)
	go func() {
		defer close(ch)
		for t.stdout.Scan() {
			line := t.stdout.Text()
			msg, err := parseStdoutLine(line)
			if err != nil {
				t.setErr(err)
				return
			}
			if msg == nil {
				continue
			}
			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			}
		}
		if err := t.stdout.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				t.setErr(&CLIJSONDecodeError{
					Line:  fmt.Sprintf("exceeded maximum buffer size of %d bytes", t.maxBufferSize),
					Cause: err,
				})
			} else {
				t.setErr(&ProcessError{Message: "stdout read error", Stderr: err.Error()})
			}
		}
	}()
	return ch
}

func (t *cliTransport) write(ctx context.Context, line string) error {
	t.stdinMu.Lock()
	defer t.stdinMu.Unlock()

	// Pre-flight checks (mirrors Python's write() guards).
	if t.writeFailed {
		return &CLIConnectionError{
			Message: fmt.Sprintf("cannot write to process that exited with error: %v", t.writeFailErr),
		}
	}
	if t.closed {
		return &CLIConnectionError{Message: "transport is closed"}
	}

	_, err := fmt.Fprintln(t.stdin, line)
	if err != nil {
		// Mark transport as permanently poisoned — no more writes will succeed.
		// Mirrors Python's _ready = False / _exit_error pattern.
		t.writeFailed = true
		t.writeFailErr = err
		return &CLIConnectionError{
			Message: fmt.Sprintf("failed to write to process stdin: %v", err),
		}
	}
	return nil
}

func (t *cliTransport) closeStdin() error {
	t.stdinMu.Lock()
	defer t.stdinMu.Unlock()
	if t.stdin != nil {
		return t.stdin.Close()
	}
	return nil
}

func (t *cliTransport) close() error {
	// Acquire stdinMu first to set closed and close stdin atomically with
	// write() and closeStdin(). This mirrors the Python SDK's _write_lock
	// which is held by both close() and write() to prevent TOCTOU races.
	t.stdinMu.Lock()
	if t.closed {
		t.stdinMu.Unlock()
		return nil
	}
	t.closed = true
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	t.stdinMu.Unlock()

	// Graceful shutdown: wait for process to exit after stdin EOF.
	// The subprocess needs time to flush its session file after receiving
	// EOF on stdin. Without this grace period, SIGTERM can interrupt the
	// write and cause the last assistant message to be lost.
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd != nil && t.cmd.Process != nil {
		// Wait up to 5s for natural exit after stdin close.
		done := make(chan error, 1)
		go func() { done <- t.cmd.Wait() }()
		select {
		case <-done:
			unregisterChild(t.cmd)
			return nil
		case <-time.After(5 * time.Second):
		}
		// SIGTERM fallback.
		_ = t.cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
			unregisterChild(t.cmd)
			return nil
		case <-time.After(5 * time.Second):
		}
		// SIGKILL fallback.
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}
	// Only stop tracking a child we actually reaped. A still-running
	// process stays in the set so the atexit reaper gets a chance at it.
	if t.cmd != nil && t.cmd.ProcessState != nil && t.cmd.ProcessState.Exited() {
		unregisterChild(t.cmd)
	}
	return nil
}

func (t *cliTransport) setErr(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err == nil {
		t.err = err
	}
}

func (t *cliTransport) checkVersion() {
	out, err := exec.Command(t.cliPath, "--version").Output()
	if err != nil {
		return
	}
	version := strings.TrimSpace(strings.TrimPrefix(string(out), "v"))
	if !versionAtLeast(version, minimumClaudeCodeVersion) {
		log.Printf("Warning: Claude Code version %s may be outdated (minimum: %s)", version, minimumClaudeCodeVersion)
	}
}

func versionAtLeast(actual, minimum string) bool {
	actual = strings.TrimPrefix(actual, "v")
	minimum = strings.TrimPrefix(minimum, "v")
	ap := strings.SplitN(actual, ".", 3)
	mp := strings.SplitN(minimum, ".", 3)
	for i := 0; i < 3; i++ {
		var a, m int
		if i < len(ap) {
			n := strings.FieldsFunc(ap[i], func(r rune) bool { return r == '-' || r == '+' })
			a, _ = strconv.Atoi(n[0])
		}
		if i < len(mp) {
			m, _ = strconv.Atoi(mp[i])
		}
		if a > m {
			return true
		}
		if a < m {
			return false
		}
	}
	return true
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func appendIfMissing(env []string, entry string) []string {
	key := strings.SplitN(entry, "=", 2)[0] + "="
	for _, e := range env {
		if strings.HasPrefix(e, key) {
			return env
		}
	}
	return append(env, entry)
}
