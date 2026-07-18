//go:build !windows

package ninerouter

import (
	"fmt"
	"os/exec"
	"time"
)

func commandForScript(path string, args []string) *exec.Cmd { return exec.Command(path, args...) }
func prepareCommand(*exec.Cmd)                              {}
func processMatches(int, time.Time) bool                    { return false }
func stopProcessTree(int) error                             { return fmt.Errorf("process control is only supported on Windows") }
