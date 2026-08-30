//go:build windows

package upgrader

import (
	"fmt"
	"os"

	"gitmake/internal/winreplace"
)

func StageReplacement(newExe string) error {
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	if _, err := winreplace.Stage(newExe, current, os.Getpid()); err != nil {
		return fmt.Errorf("stage self-replacement: %w", err)
	}
	return nil
}
