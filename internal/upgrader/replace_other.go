//go:build !windows

package upgrader

import "fmt"

func StageReplacement(string) error {
	return fmt.Errorf("gitmake upgrade self-replacement is currently supported on Windows only")
}
