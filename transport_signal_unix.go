//go:build !windows

package claude

import (
	"os"
	"os/signal"
	"syscall"
)

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

// terminateSignal returns the appropriate signal for graceful termination.
// On Unix, this is SIGTERM. On Windows, this is os.Interrupt (syscall.SIGTERM is a no-op on Windows).
var terminateSignal = syscall.SIGTERM
