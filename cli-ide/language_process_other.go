//go:build !linux

package main

import "os/exec"

func isolateLanguageServer(*exec.Cmd) {}
