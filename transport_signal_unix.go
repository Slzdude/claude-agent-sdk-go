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
