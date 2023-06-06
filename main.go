package main

import (
	"os/exec"
	"stkey/cmd"
	"stkey/pkg/logger"
)

func main() {
	logger.Init()
	_ = exec.Command("/bin/bash", "-c", "ulimit -u 65535").Run()
	_ = exec.Command("/bin/bash", "-c", "ulimit -n 65535").Run()
	cmd.Execute()
}
