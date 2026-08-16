//go:build unix

package verify

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the verify command in a fresh process group so the
// whole tree it spawns can be signalled as a unit.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup SIGKILLs the command's entire process group. Negating the
// pid addresses the group rather than the single process, which is the point:
// a timed-out verify that already forked children leaves them running when
// only the direct child is killed.
//
// Falls back to killing the process alone if the group id cannot be resolved,
// so a partial failure still terminates something rather than nothing.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return cmd.Process.Kill()
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
