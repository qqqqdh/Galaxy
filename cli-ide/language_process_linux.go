package main

import (
	"os"
	"os/exec"
	"syscall"
)

func isolateLanguageServer(cmd *exec.Cmd) {
	// A terminal interrupt should reach the foreground task while the editor keeps its language session.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func isolateTestCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}
