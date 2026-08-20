package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gitmake/internal/config"
)

type initPrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func newInitPrompter() initPrompter {
	return initPrompter{in: bufio.NewReader(os.Stdin), out: os.Stdout}
}

func (p initPrompter) line(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	raw, err := p.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		value = def
	}
	return value, nil
}

func (p initPrompter) yesNo(label string, def bool) (bool, error) {
	suffix := "[Y/n]"
	if !def {
		suffix = "[y/N]"
	}
	for {
		fmt.Fprintf(p.out, "%s %s: ", label, suffix)
		raw, err := p.in.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			return def, nil
		}
		switch value {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.out, "  Enter y or n.")
		}
	}
}

func (p initPrompter) visibility(def string) (string, error) {
	fmt.Fprintln(p.out, "Visibility")
	fmt.Fprintln(p.out, "  1) Private")
	fmt.Fprintln(p.out, "  2) Public")
	fmt.Fprintln(p.out, "  3) Internal")
	defaultChoice := "1"
	switch strings.ToLower(def) {
	case "public":
		defaultChoice = "2"
	case "internal":
		defaultChoice = "3"
	}
	for {
		choice, err := p.line("Select", defaultChoice)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1", "private":
			return "private", nil
		case "2", "public":
			return "public", nil
		case "3", "internal":
			return "internal", nil
		default:
			fmt.Fprintln(p.out, "  Choose 1, 2, or 3.")
		}
	}
}

func (p initPrompter) chooseZIP(zips []string) (string, error) {
	fmt.Fprintln(p.out, "Project ZIP")
	for i, z := range zips {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, z)
	}
	for {
		choice, err := p.line("Select", "1")
		if err != nil {
			return "", err
		}
		var n int
		if _, err := fmt.Sscanf(choice, "%d", &n); err == nil && n >= 1 && n <= len(zips) {
			return zips[n-1], nil
		}
		fmt.Fprintf(p.out, "  Choose a number from 1 to %d.\n", len(zips))
	}
}

func runInitWizard(cfg config.Config, configPath, zipName string, assumeYes bool) error {
	fmt.Printf("GitMake setup · %s\n\n", Version)
	fmt.Printf("✓ Source      %s\n\n", zipName)

	if assumeYes {
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}
		printInitSummary(cfg, configPath)
		return nil
	}

	p := newInitPrompter()
	repoName, err := p.line("Repository name", cfg.Repo.Name)
	if err != nil {
		return err
	}
	cfg.Repo.Name = repoName

	visibility, err := p.visibility(cfg.Repo.Visibility)
	if err != nil {
		return err
	}
	cfg.Repo.Visibility = visibility

	description, err := p.line("Description (optional)", cfg.Repo.Description)
	if err != nil {
		return err
	}
	cfg.Repo.Description = description

	branch, err := p.line("Default branch", cfg.Git.Branch)
	if err != nil {
		return err
	}
	cfg.Git.Branch = branch

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("setup values are invalid: %w", err)
	}

	fmt.Fprintln(p.out)
	fmt.Fprintf(p.out, "  Repository  %s · %s\n", cfg.Repo.Name, cfg.Repo.Visibility)
	fmt.Fprintf(p.out, "  Branch      %s\n", cfg.Git.Branch)
	if cfg.Repo.Description != "" {
		fmt.Fprintf(p.out, "  Description %s\n", cfg.Repo.Description)
	}
	fmt.Fprintln(p.out)

	ok, err := p.yesNo("Create gitmake.json?", true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(p.out, "\nSetup cancelled. No files were changed.")
		return nil
	}
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}
	printInitSummary(cfg, configPath)
	return nil
}

func printInitSummary(cfg config.Config, configPath string) {
	fmt.Println()
	fmt.Printf("✓ Repository  %s · %s\n", cfg.Repo.Name, cfg.Repo.Visibility)
	fmt.Printf("✓ Config      %s\n", configPath)
	fmt.Println("\nReady. Run `gitmake` to publish.")
}
