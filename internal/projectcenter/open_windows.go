//go:build windows

package projectcenter

import (
	"fmt"
	"os/exec"
)

func openFolder(path string) error {
	if err := exec.Command("explorer.exe", path).Start(); err != nil {
		return fmt.Errorf("open project folder: %w", err)
	}
	return nil
}
