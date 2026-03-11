//go:build windows

package claude

import "os/exec"

// setCmdUser is a no-op on Windows; subprocess user switching is not supported.
func setCmdUser(cmd *exec.Cmd, username string) error {
	return nil
}
