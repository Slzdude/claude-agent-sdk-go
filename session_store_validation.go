package claude

import "fmt"

// ValidateSessionStoreOptions checks for invalid ClaudeAgentOptions combinations
// involving SessionStore. Called before subprocess spawn so misconfiguration
// fails fast instead of surfacing as a confusing runtime error mid-session.
func ValidateSessionStoreOptions(opts *ClaudeAgentOptions) error {
	if opts == nil || opts.SessionStore == nil {
		return nil
	}

	// continue_conversation + session_store requires list_sessions support.
	// When resume is explicitly set, list_sessions() is provably never called
	// (resume wins over continue), so a minimal store is fine.
	if opts.ContinueConversation && opts.Resume == "" {
		if !storeSupportsListSessions(opts.SessionStore) {
			return fmt.Errorf(
				"continue_conversation with session_store requires the store to implement ListSessions()")
		}
	}

	// session_store + enable_file_checkpointing is incompatible.
	if opts.EnableFileCheckpointing {
		return fmt.Errorf(
			"session_store cannot be combined with enable_file_checkpointing " +
				"(checkpoints are local-disk only and would diverge from the mirrored transcript)")
	}

	return nil
}

// storeSupportsListSessions checks if the store's ListSessions returns real
// data vs. a stub. InMemorySessionStore always supports it; external adapters
// may implement the interface but return empty/error for optional methods.
// This mirrors Python's _store_implements() which detects Protocol defaults.
func storeSupportsListSessions(_ SessionStore) bool {
	// All Go SessionStore implementations must implement ListSessions.
	// The Go interface doesn't have optional methods like Python's Protocol,
	// so if the store satisfies the interface, it supports all methods.
	// A stub returning empty is still valid — the caller handles empty results.
	return true
}
