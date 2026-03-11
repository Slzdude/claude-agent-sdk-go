//go:build !windows

package claude

import (
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// setCmdUser configures cmd to run as the named OS user.
// It looks up the user, resolves the UID/GID, and sets SysProcAttr.Credential.
func setCmdUser(cmd *exec.Cmd, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}
	return nil
}
