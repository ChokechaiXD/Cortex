//go:build windows

package ninerouter

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func commandForScript(path string, args []string) *exec.Cmd {
	if strings.EqualFold(filepathExt(path), ".cmd") || strings.EqualFold(filepathExt(path), ".bat") {
		quoted := `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
		return exec.Command("cmd.exe", append([]string{"/d", "/s", "/c", quoted}, args...)...)
	}
	return exec.Command(path, args...)
}

func prepareCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func processMatches(pid int, startedAt time.Time) bool {
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

func stopProcessTree(pid int) error {
	command := exec.Command("taskkill.exe", "/PID", strconv.Itoa(pid), "/T", "/F")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("stop managed process: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func filepathExt(path string) string {
	index := strings.LastIndex(path, ".")
	if index < 0 {
		return ""
	}
	return path[index:]
}
