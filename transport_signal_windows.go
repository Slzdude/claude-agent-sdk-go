//go:build windows

package claude

import "os"

// No signal handler on Windows. Windows doesn't reliably support
// SIGTERM, and signal.Notify with os.Interrupt may interfere with
// test cleanup. The atexit reaper handles orphaned processes.

// terminateSignal returns os.Interrupt on Windows because syscall.SIGTERM
// is a no-op on Windows (Process.Signal only supports os.Interrupt and os.Kill).
var terminateSignal = os.Interrupt
