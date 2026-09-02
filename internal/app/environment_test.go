package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// The human and machine views of doctor/install/upgrade are rendered from one
// report, so these tests pin the report shape an agent consumes and the
// wording a user reads.

func TestDoctorReportCountsIssuesAndIgnoresAdvisories(t *testing.T) {
	r := DoctorReport{
		Schema:  "gitmake.doctor/v1",
		Version: Version,
		Checks: []DoctorCheck{
			{Name: "Git", OK: true, Detail: "git version 2.x"},
			{Name: "GitHub login", OK: false, Detail: "not signed in"},
			{Name: "Project config", OK: true, Detail: "not present (optional)", Advisory: true},
		},
		Issues:   1,
		Remedies: []string{"gh auth login"},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema", "version", "ok", "issues", "checks"} {
		if _, present := decoded[field]; !present {
			t.Fatalf("doctor JSON is missing %q; agents read this contract", field)
		}
	}
	if decoded["schema"] != "gitmake.doctor/v1" {
		t.Fatalf("schema = %v", decoded["schema"])
	}
}

func TestCollectDoctorReportIsSelfConsistent(t *testing.T) {
	r := collectDoctorReport(Options{})
	if r.Schema != "gitmake.doctor/v1" || r.Version != Version {
		t.Fatalf("unexpected header: %+v", r)
	}
	if len(r.Checks) == 0 {
		t.Fatal("doctor reported no checks at all")
	}

	failing := 0
	for _, c := range r.Checks {
		if c.Advisory {
			if !c.OK {
				t.Fatalf("advisory check %q reported a failure; advisories carry context, not verdicts", c.Name)
			}
			continue
		}
		if !c.OK {
			failing++
		}
		if c.Detail == "" {
			t.Fatalf("check %q has no detail", c.Name)
		}
	}
	if failing != r.Issues {
		t.Fatalf("issues = %d but %d checks failed", r.Issues, failing)
	}
	if r.OK != (r.Issues == 0) {
		t.Fatalf("ok = %v with %d issues", r.OK, r.Issues)
	}
	if !r.OK && len(r.Remedies) == 0 {
		t.Fatal("a failing environment must offer at least one remedy")
	}
}

// TestUpgradeReportNeverClaimsAnInstallThatDidNotHappen is the reporting
// invariant that v1.2.6 introduced: "Installed" is only for a replacement
// already on disk.
func TestUpgradeReportNeverClaimsAnInstallThatDidNotHappen(t *testing.T) {
	cases := []struct {
		name    string
		report  UpgradeReport
		wants   []string
		forbids []string
	}{
		{
			name:    "up to date",
			report:  UpgradeReport{CurrentVersion: "1.2.7", LatestTag: "v1.2.7"},
			wants:   []string{"up to date"},
			forbids: []string{"Installed", "scheduled"},
		},
		{
			name:    "check only, update available",
			report:  UpgradeReport{CurrentVersion: "1.2.6", LatestTag: "v1.2.7", UpdateAvailable: true, CheckOnly: true},
			wants:   []string{"Update available", "Nothing was downloaded or replaced"},
			forbids: []string{"Installed", "SHA-256"},
		},
		{
			name:    "installed",
			report:  UpgradeReport{CurrentVersion: "1.2.6", LatestTag: "v1.2.7", UpdateAvailable: true, Installed: true, Target: "/tmp/gitmake"},
			wants:   []string{"Downloaded v1.2.7", "SHA-256 verified", "Installed v1.2.7"},
			forbids: []string{"scheduled"},
		},
		{
			name:    "scheduled",
			report:  UpgradeReport{CurrentVersion: "1.2.6", LatestTag: "v1.2.7", UpdateAvailable: true, ReplacementScheduled: true, Target: "/tmp/gitmake", HelperLog: "/tmp/log"},
			wants:   []string{"scheduled after this process exits", "/tmp/log"},
			forbids: []string{"✓ Installed"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := captureOutput(func() error {
				renderUpgradeReport(tc.report, false)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q:\n%s", want, out)
				}
			}
			for _, forbidden := range tc.forbids {
				if strings.Contains(out, forbidden) {
					t.Fatalf("output must not contain %q:\n%s", forbidden, out)
				}
			}
		})
	}
}

func TestInstallReportDistinguishesScheduledFromInstalled(t *testing.T) {
	done, err := captureOutput(func() error {
		renderInstallReport(InstallReport{OK: true, Target: "/tmp/gitmake", PathAdded: false})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(done, "✓ Installed") {
		t.Fatalf("completed install should say so:\n%s", done)
	}

	pending, err := captureOutput(func() error {
		renderInstallReport(InstallReport{OK: true, Target: "/tmp/gitmake", ReplacementScheduled: true, ReplacementLog: "/tmp/log"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pending, "✓ Installed") {
		t.Fatalf("a scheduled replacement must not report an install:\n%s", pending)
	}
	if !strings.Contains(pending, "/tmp/log") {
		t.Fatalf("scheduled replacement should name its log:\n%s", pending)
	}
}

func TestCheckFlagIsRejectedOutsideUpgrade(t *testing.T) {
	if _, err := parseArgs([]string{"upgrade", "--check"}); err != nil {
		t.Fatalf("--check should be valid with upgrade: %v", err)
	}
	for _, command := range []string{"doctor", "install"} {
		if _, err := parseArgs([]string{command, "--check"}); err == nil {
			t.Fatalf("--check should be rejected with %q", command)
		}
	}
}
