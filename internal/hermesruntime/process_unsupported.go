//go:build !windows

package hermesruntime

import (
	"os/exec"
	"time"
)

func prepareCommand(*exec.Cmd) {}

func ProcessMatches(pid int, startedAt time.Time) bool {
	return pid > 0 && !startedAt.IsZero()
}
