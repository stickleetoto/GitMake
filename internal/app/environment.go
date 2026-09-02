package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/installer"
	"gitmake/internal/runner"
)

// GitMake is an AI-first tool, but `doctor`, `install`, and `upgrade` printed
// only for humans: an agent had to parse prose to learn whether the
// environment was usable or whether an upgrade had actually landed. These
// reports give each of those commands a machine surface, and the human output
// is rendered from the same data so the two cannot drift.

// DoctorCheck is one environment check. Advisory entries carry context rather
// than a pass/fail verdict and never count as issues.
type DoctorCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
	Advisory bool   `json:"advisory,omitempty"`
}

type DoctorReport struct {
	Schema   string        `json:"schema"`
	Version  string        `json:"version"`
	OK       bool          `json:"ok"`
	Issues   int           `json:"issues"`
	Checks   []DoctorCheck `json:"checks"`
	Remedies []string      `json:"remedies,omitempty"`
}

type InstallReport struct {
	Schema string `json:"schema"`
	OK     bool   `json:"ok"`
	Target string `json:"target"`
	// ReplacementScheduled means the executable was in use and a helper will
	// finish after this process exits. It is never reported as installed.
	ReplacementScheduled bool   `json:"replacement_scheduled"`
	ReplacementLog       string `json:"replacement_log,omitempty"`
	PathAdded            bool   `json:"path_added"`
}

type UpgradeReport struct {
	Schema          string `json:"schema"`
	OK              bool   `json:"ok"`
	CurrentVersion  string `json:"current_version"`
	LatestTag       string `json:"latest_tag"`
	UpdateAvailable bool   `json:"update_available"`
	// CheckOnly reports that nothing was downloaded or replaced.
	CheckOnly bool `json:"check_only"`
	Installed bool `json:"installed"`
	// ReplacementScheduled means the new build is downloaded and verified but
	// not yet in place.
	ReplacementScheduled bool   `json:"replacement_scheduled"`
	Target               string `json:"target,omitempty"`
	PreviousImage        string `json:"previous_image,omitempty"`
	HelperLog            string `json:"helper_log,omitempty"`
}

func collectDoctorReport(o Options) DoctorReport {
	report := DoctorReport{Schema: "gitmake.doctor/v1", Version: Version}
	add := func(name string, ok bool, detail string) {
		report.Checks = append(report.Checks, DoctorCheck{Name: name, OK: ok, Detail: detail})
		if !ok {
			report.Issues++
		}
	}
	note := func(name, detail string) {
		report.Checks = append(report.Checks, DoctorCheck{Name: name, OK: true, Detail: detail, Advisory: true})
	}

	run := runner.Runner{Verbose: o.Verbose}

	gitVer, err := run.Run("", "git", "--version")
	add("Git", err == nil && gitVer.Code == 0, firstNonEmpty(gitVer.Stdout, "not found"))

	ghVer, err := run.Run("", "gh", "--version")
	ghOK := err == nil && ghVer.Code == 0
	add("GitHub CLI", ghOK, firstLine(firstNonEmpty(ghVer.Stdout, "not found")))

	login := "not signed in"
	authOK := false
	if ghOK {
		if res, e := run.Run("", "gh", "auth", "status", "--hostname", "github.com"); e == nil && res.Code == 0 {
			authOK = true
			if u, e2 := run.Run("", "gh", "api", "user", "--jq", ".login"); e2 == nil && u.Code == 0 {
				login = strings.TrimSpace(u.Stdout)
			} else {
				login = "signed in"
			}
		}
	}
	add("GitHub login", authOK, login)

	name, _ := run.Run("", "git", "config", "--global", "--get", "user.name")
	email, _ := run.Run("", "git", "config", "--global", "--get", "user.email")
	identOK := name.Code == 0 && email.Code == 0 && strings.TrimSpace(name.Stdout) != "" && strings.TrimSpace(email.Stdout) != ""
	ident := "not configured"
	if identOK {
		ident = strings.TrimSpace(name.Stdout) + " <" + strings.TrimSpace(email.Stdout) + ">"
	}
	add("Git identity", identOK, ident)

	pathStatus := installer.GetPathStatus()
	installDetail := pathStatus.InstallTarget
	if installDetail == "" {
		installDetail = "installation target unavailable"
	}
	if !pathStatus.InstalledBinary && pathStatus.InstallTarget != "" {
		installDetail = "not installed (target: " + pathStatus.InstallTarget + ")"
	}
	add("GitMake install", pathStatus.InstalledBinary, installDetail)
	add("CLI command", pathStatus.Healthy(), pathStatusDetail(pathStatus))

	if pathStatus.InstalledBinary && pathStatus.UserPathHasInstall && !pathStatus.CommandAvailable && !pathStatus.CurrentIsInstalledCopy {
		note("Current shell", "user PATH is registered; reopen the terminal to refresh command resolution")
	}
	if pathStatus.ResolvedPath != "" && pathStatus.InstallTarget != "" && !samePathForDisplay(pathStatus.ResolvedPath, pathStatus.InstallTarget) {
		note("Resolved copy", pathStatus.ResolvedPath+" (different from the standard install target)")
	}

	cwd, _ := os.Getwd()
	if configPath := filepath.Join(cwd, "gitmake.json"); fileExists(configPath) {
		add("Project config", true, configPath)
	} else {
		note("Project config", "not present in this folder (optional)")
	}

	if !authOK && ghOK {
		report.Remedies = append(report.Remedies, "gh auth login")
	}
	if !identOK {
		report.Remedies = append(report.Remedies, "git config --global user.name ... and user.email ...")
	}
	if !pathStatus.Healthy() || !pathStatus.InstalledBinary {
		report.Remedies = append(report.Remedies, "gitmake install")
	}
	report.OK = report.Issues == 0
	return report
}

func renderInstallReport(r InstallReport) {
	fmt.Printf("GitMake %s · Install\n\n", Version)
	if r.ReplacementScheduled {
		// Never claim an install that has not happened yet.
		fmt.Println("· Replacement scheduled after this process exits")
		fmt.Println("  " + r.Target)
		if r.ReplacementLog != "" {
			fmt.Println("  Log: " + r.ReplacementLog)
		}
		fmt.Println()
		fmt.Println("  Verify once this command has closed:")
		fmt.Printf("  \"%s\" --version\n", r.Target)
	} else {
		fmt.Println("✓ Installed")
		fmt.Println("  " + r.Target)
	}
	if r.PathAdded {
		fmt.Println("✓ Added to your user PATH")
		fmt.Println("\nOpen a new PowerShell/Terminal window, then run:\n  gitmake doctor")
	} else {
		fmt.Println("✓ User PATH already contains GitMake")
	}
}

func renderUpgradeReport(r UpgradeReport, verbose bool) {
	fmt.Printf("GitMake %s · Upgrade\n\n", r.CurrentVersion)
	if verbose {
		fmt.Println("$ public GitHub Release API")
	}

	if r.CheckOnly {
		if r.UpdateAvailable {
			fmt.Printf("· Update available: %s (current %s)\n", r.LatestTag, r.CurrentVersion)
			fmt.Println("\nRun: gitmake upgrade")
		} else {
			fmt.Printf("✓ GitMake is up to date (current %s, GitHub latest %s)\n", r.CurrentVersion, r.LatestTag)
		}
		fmt.Println("\nNothing was downloaded or replaced.")
		return
	}

	if !r.Installed && !r.ReplacementScheduled {
		fmt.Printf("✓ GitMake is up to date (current %s, GitHub latest %s)\n", r.CurrentVersion, r.LatestTag)
		return
	}

	fmt.Printf("✓ Downloaded %s\n", r.LatestTag)
	fmt.Println("✓ SHA-256 verified")
	if r.ReplacementScheduled {
		fmt.Println("· Replacement scheduled after this process exits")
		fmt.Printf("  %s\n", r.Target)
		if r.HelperLog != "" {
			fmt.Printf("  Log: %s\n", r.HelperLog)
		}
		fmt.Println()
		fmt.Println("  Verify once this command has closed:")
		fmt.Printf("  \"%s\" --version\n", r.Target)
		return
	}

	fmt.Printf("✓ Installed %s\n", r.LatestTag)
	fmt.Printf("  %s\n", r.Target)
	if r.PreviousImage != "" {
		fmt.Println("  The previous build is still running elsewhere; its file is removed on a later run.")
	}
	if dir := installer.InstallDir(); dir != "" {
		installed := filepath.Join(dir, "gitmake"+exeSuffix())
		if !samePathForDisplay(r.Target, installed) {
			// Upgrading a copy outside the install directory is legitimate,
			// but the user must not believe the PATH command was updated.
			fmt.Printf("\n! This is not the installed copy at %s\n", installed)
			fmt.Println("  Run 'gitmake install' from the upgraded executable to update the PATH command.")
		}
	}
	fmt.Println("\nRun: gitmake --version")
}

func renderDoctorReport(r DoctorReport) {
	fmt.Printf("GitMake Doctor · %s\n\n", r.Version)
	for _, c := range r.Checks {
		switch {
		case c.Advisory:
			fmt.Printf("· %-16s %s\n", c.Name, c.Detail)
		case c.OK:
			fmt.Printf("✓ %-16s %s\n", c.Name, c.Detail)
		default:
			fmt.Printf("× %-16s %s\n", c.Name, c.Detail)
		}
	}
	fmt.Println()
	if r.OK {
		fmt.Println("Everything looks good.")
		return
	}
	fmt.Printf("%d issue(s) found.\n", r.Issues)
	for _, remedy := range r.Remedies {
		switch {
		case strings.HasPrefix(remedy, "gh auth"):
			fmt.Println("Run: " + remedy)
		case strings.HasPrefix(remedy, "git config"):
			fmt.Println("Set: " + remedy)
		default:
			fmt.Println("Install: " + remedy)
		}
	}
}
