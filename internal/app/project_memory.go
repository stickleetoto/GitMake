package app

import (
	"fmt"
	"strings"

	"gitmake/internal/config"
	"gitmake/internal/projectid"
)

func applyFolderProjectMemory(sel sourceSelection, cfg *config.Config) (bool, error) {
	if sel.Mode != "folder" || cfg == nil {
		return false, nil
	}
	record, exists, err := projectid.Read(sel.Path)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	parts := strings.SplitN(strings.TrimSpace(record.Repository), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false, fmt.Errorf("project memory repository is invalid: %q", record.Repository)
	}
	cfg.Repo.Owner = parts[0]
	cfg.Repo.Name = parts[1]
	return true, nil
}

func validateFolderProjectMemory(sel sourceSelection, repository string) error {
	if sel.Mode != "folder" {
		return nil
	}
	_, _, err := projectid.Validate(sel.Path, repository)
	return err
}

func rememberFolderProject(sel sourceSelection, repository string) error {
	if sel.Mode != "folder" {
		return nil
	}
	_, err := projectid.Write(sel.Path, repository)
	if err != nil {
		return fmt.Errorf("remember project repository: %w", err)
	}
	return nil
}
