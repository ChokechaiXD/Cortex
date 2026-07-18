//go:build windows

package hermesruntime

import (
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func prepareCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func ProcessMatches(pid int, startedAt time.Time) bool {
	if pid <= 0 || startedAt.IsZero() {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return false
	}
	actual := time.Unix(0, created.Nanoseconds()).UTC()
	delta := actual.Sub(startedAt.UTC())
	if delta < 0 {
		delta = -delta
	}
	return delta < 5*time.Second
}
