package app

import (
	"strings"
	"testing"
)

func TestReplaceManagedSectionPreservesUserContent(t *testing.T) {
	existing := "# Project\n\nUser rules.\n\n" + agentsBegin + "\nold\n" + agentsEnd + "\n\nTail.\n"
	got := replaceManagedSection(existing, managedAgentSection())
	if !strings.Contains(got, "# Project") || !strings.Contains(got, "User rules.") || !strings.Contains(got, "Tail.") {
		t.Fatalf("user content lost: %q", got)
	}
	if strings.Count(got, agentsBegin) != 1 || strings.Count(got, agentsEnd) != 1 {
		t.Fatalf("managed section should be unique: %q", got)
	}
	if !strings.Contains(got, "gitmake --dry-run --read-only --json") {
		t.Fatalf("new managed instructions missing: %q", got)
	}
}

func TestReplaceManagedSectionIsIdempotent(t *testing.T) {
	first := replaceManagedSection("# Existing\n", managedAgentSection())
	second := replaceManagedSection(first, managedAgentSection())
	if first != second {
		t.Fatalf("not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestAIManifestSafeDefaults(t *testing.T) {
	m := aiManifest()
	if m.Schema != "gitmake.ai/v1" || m.Version != Version {
		t.Fatalf("unexpected manifest identity: %#v", m)
	}
	if m.Safety.ForcePush || m.Safety.RewriteHistory || m.Safety.DeleteRepositories {
		t.Fatalf("unsafe capability advertised: %#v", m.Safety)
	}
	preview := m.Commands["preview"].Command
	if !strings.Contains(preview, "--dry-run") || !strings.Contains(preview, "--read-only") || !strings.Contains(preview, "--json") {
		t.Fatalf("preview command is not safely machine-readable: %q", preview)
	}
}
