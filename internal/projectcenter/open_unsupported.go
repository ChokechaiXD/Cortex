//go:build !windows

package projectcenter

import "fmt"

func openFolder(string) error {
	return fmt.Errorf("opening project folders is supported on Windows only")
}
