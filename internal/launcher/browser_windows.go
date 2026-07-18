//go:build windows

package launcher

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

func Open(_ context.Context, rawURL string) error {
	if err := validateDashboardURL(rawURL); err != nil {
		return err
	}
	if browser, ok := appBrowser(); ok {
		return startHidden(exec.Command(browser, "--app="+rawURL, "--window-size=1440,920"), "launch HOPE app")
	}
	return startHidden(exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", rawURL), "launch browser")
}

func appBrowser() (string, bool) {
	for _, name := range []string{"msedge.exe", "chrome.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}

func startHidden(command *exec.Cmd, label string) error {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x08000000}
	if err := command.Start(); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return command.Process.Release()
}
