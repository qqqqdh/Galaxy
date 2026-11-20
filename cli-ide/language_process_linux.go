package main

import (
	"os/exec"
	"syscall"
)

func isolateLanguageServer(cmd *exec.Cmd) {
	// A terminal interrupt should reach the foreground task while the editor keeps its language session.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
