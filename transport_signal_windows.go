//go:build windows

package claude

// No signal handler on Windows. Windows doesn't reliably support
// SIGTERM, and signal.Notify with os.Interrupt may interfere with
// test cleanup. The atexit reaper handles orphaned processes.
