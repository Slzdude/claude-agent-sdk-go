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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultMaxBufferSize     = 1024 * 1024
	minimumClaudeCodeVersion = "2.0.0"
	sdkVersion               = "0.1.0"
)

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
		cmd = append(cmd, "--system-prompt", "")
	case string:
		cmd = append(cmd, "--system-prompt", sp)
	case *SystemPromptPreset:
		if sp.Append != "" {
			cmd = append(cmd, "--append-system-prompt", sp.Append)
		}
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

	if len(opts.AllowedTools) > 0 {
		cmd = append(cmd, "--allowedTools", strings.Join(opts.AllowedTools, ","))
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
		cmd = append(cmd, "--resume", opts.Resume)
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
	// --setting-sources is always emitted (even when empty) to match Python SDK
	// behaviour: this tells the CLI exactly which settings tiers to load.
	{
		var sourceParts []string
		if opts.SettingSources != nil {
			sourceParts = make([]string, len(opts.SettingSources))
			for i, s := range opts.SettingSources {
				sourceParts[i] = string(s)
			}
		}
		cmd = append(cmd, "--setting-sources", strings.Join(sourceParts, ","))
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

	// Thinking
	resolvedThinking := -1
	if opts.MaxThinkingTokens != nil {
		resolvedThinking = *opts.MaxThinkingTokens
	}
	if opts.Thinking != nil {
		switch th := opts.Thinking.(type) {
		case *ThinkingAdaptive:
			if resolvedThinking < 0 {
				resolvedThinking = 32_000
			}
		case *ThinkingEnabled:
			resolvedThinking = th.BudgetTokens
		case *ThinkingDisabled:
			resolvedThinking = 0
		}
	}
	if resolvedThinking >= 0 {
		cmd = append(cmd, "--max-thinking-tokens", strconv.Itoa(resolvedThinking))
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
	hasSandbox := len(opts.Sandbox) > 0
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
		obj["sandbox"] = map[string]any(opts.Sandbox)
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

	env := os.Environ()
	for k, v := range t.opts.Env {
		env = append(env, k+"="+v)
	}
	env = append(env, "CLAUDE_CODE_ENTRYPOINT=sdk-go", "CLAUDE_AGENT_SDK_VERSION="+sdkVersion)
	if t.opts.EnableFileCheckpointing {
		env = append(env, "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
	}
	if t.opts.IncludePartialMessages {
		env = appendIfMissing(env, "CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING=1")
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

	var stderrPipe io.ReadCloser
	_, hasDebugStderr := t.opts.ExtraArgs["debug-to-stderr"]
	if t.opts.Stderr != nil || hasDebugStderr {
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
			t.opts.Stderr(line)
		}
	}
}

// readMessages delivers raw JSON objects from stdout into a channel.
func (t *cliTransport) readMessages(ctx context.Context) <-chan map[string]any {
	ch := make(chan map[string]any, 64)
	go func() {
		defer close(ch)
		for t.stdout.Scan() {
			line := t.stdout.Text()
			if line == "" {
				continue
			}
			var msg map[string]any
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				t.setErr(&CLIJSONDecodeError{Line: line, Cause: err})
				return
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
	if t.closed {
		return fmt.Errorf("transport is closed")
	}
	_, err := fmt.Fprintln(t.stdin, line)
	return err
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
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
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

func appendIfMissing(env []string, entry string) []string {
	key := strings.SplitN(entry, "=", 2)[0] + "="
	for _, e := range env {
		if strings.HasPrefix(e, key) {
			return env
		}
	}
	return append(env, entry)
}
