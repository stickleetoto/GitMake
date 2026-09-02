package securityscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures are assembled from fragments so this source file never contains a
// literal credential signature; a real one here would block GitMake's own
// publish. TestTestSourcesDoNotContainLiteralSecretSignatures enforces that.

func scanOne(t *testing.T, name, body string) Report {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(root, Options{SecretScan: true})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func kinds(r Report) map[string]Finding {
	out := map[string]Finding{}
	for _, f := range r.Findings {
		out[f.Kind] = f
	}
	return out
}

// TestDetectsCommonProviderCredentials covers the credentials that an
// AI-authored project actually leaks. Before this the scanner knew five
// shapes, so a model provider key, a cloud service account, or a payment key
// published cleanly.
func TestDetectsCommonProviderCredentials(t *testing.T) {
	cases := []struct {
		kind  string
		value string
	}{
		{"anthropic_api_key", "sk-" + "ant-api03-" + strings.Repeat("A1b2", 8)},
		{"openai_api_key", "sk-" + "proj-" + strings.Repeat("Xy9z", 12)},
		{"huggingface_token", "hf_" + strings.Repeat("aBcD", 9)},
		{"google_api_key", "AIza" + strings.Repeat("Sy0b", 8) + "abc"},
		{"google_oauth_client_secret", "GOC" + "SPX-" + strings.Repeat("q7Wq", 6)},
		{"stripe_secret_key", "sk_" + "live_" + strings.Repeat("Zt4R", 6)},
		{"sendgrid_api_key", "SG." + strings.Repeat("aB1c", 6) + "." + strings.Repeat("dE2f", 6)},
		{"npm_token", "npm_" + strings.Repeat("k3Mn", 9)},
		{"azure_storage_key", "AccountKey=" + strings.Repeat("bXk9", 16) + "=="},
		{"slack_webhook", "https://hooks.slack.com/services/" + strings.Repeat("T0aB", 6)},
		{"discord_webhook", "https://discord.com/api/webhooks/1234567890123/" + strings.Repeat("nQ7x", 6)},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			r := scanOne(t, "leak.txt", "value = \""+tc.value+"\"\n")
			if !r.Blocking {
				t.Fatalf("%s did not block the publish", tc.kind)
			}
			found := kinds(r)
			if _, ok := found[tc.kind]; !ok {
				t.Fatalf("%s not detected; findings: %#v", tc.kind, r.Findings)
			}
		})
	}
}

func TestDetectsGCPServiceAccountJSON(t *testing.T) {
	body := "{\n  \"type\": \"service_account\",\n  \"project_id\": \"demo\"\n}\n"
	r := scanOne(t, "sa.json", body)
	if _, ok := kinds(r)["gcp_service_account"]; !ok {
		t.Fatalf("service account JSON not detected: %#v", r.Findings)
	}
}

// TestAnthropicKeyIsNotAlsoReportedAsOpenAI keeps the two prefixes from
// producing a duplicate, misleading finding for the same value.
func TestAnthropicKeyIsNotAlsoReportedAsOpenAI(t *testing.T) {
	value := "sk-" + "ant-api03-" + strings.Repeat("A1b2", 10)
	r := scanOne(t, "leak.txt", value)
	found := kinds(r)
	if _, ok := found["anthropic_api_key"]; !ok {
		t.Fatal("anthropic key not detected")
	}
	if _, ok := found["openai_api_key"]; ok {
		t.Fatal("an Anthropic key must not also be reported as an OpenAI key")
	}
}

func TestConnectionStringPasswordIsDetected(t *testing.T) {
	// Assembled from fragments: a literal connection string here is a real
	// signature and would block GitMake's own publish.
	secret := "postgres://appuser:" + "hunter2" + "SecretPw" + "@db.internal:5432/app"
	r := scanOne(t, "config.txt", "DATABASE_URL="+secret+"\n")
	found := kinds(r)
	f, ok := found["connection_string_password"]
	if !ok {
		t.Fatalf("embedded password not detected: %#v", r.Findings)
	}
	if f.Confidence != ConfidenceMedium {
		t.Fatalf("confidence = %q, want %q", f.Confidence, ConfidenceMedium)
	}
	if !r.Blocking {
		t.Fatal("a medium-confidence finding must still block; the scanner is fail-closed")
	}
}

// TestDocumentationPlaceholdersDoNotBlock matters as much as detection: a
// scanner that blocks every README example gets switched off.
func TestDocumentationPlaceholdersDoNotBlock(t *testing.T) {
	for _, line := range []string{
		"postgres://user:password@localhost:5432/db",
		"mysql://root:changeme@127.0.0.1/app",
		"redis://default:<your-password>@cache:6379",
		"postgres://user:${DB_PASSWORD}@host/db",
		"mongodb://admin:xxxxx@cluster0/test",
	} {
		r := scanOne(t, "README.md", "Connect with `"+line+"`\n")
		if len(r.Findings) != 0 {
			t.Fatalf("placeholder %q was reported as a secret: %#v", line, r.Findings)
		}
	}
}

func TestFindingsCarryConfidenceAndLine(t *testing.T) {
	body := "first line\nsecond line\nkey = \"" + "AKIA" + "ABCDEFGHIJKLMNOP" + "\"\n"
	r := scanOne(t, "app.py", body)
	f, ok := kinds(r)["aws_access_key"]
	if !ok {
		t.Fatalf("aws key not detected: %#v", r.Findings)
	}
	if f.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q, want %q", f.Confidence, ConfidenceHigh)
	}
	if f.Line != 3 {
		t.Fatalf("line = %d, want 3; remediation needs the location", f.Line)
	}
	if !strings.Contains(f.Detail, "line 3") {
		t.Fatalf("detail should name the line, got %q", f.Detail)
	}
}

// TestSecretInALargeFileIsFound covers the gap where any file over 2 MiB had
// its contents skipped entirely, so a credential in a log or database dump was
// never looked at.
func TestSecretInALargeFileIsFound(t *testing.T) {
	root := t.TempDir()
	filler := strings.Repeat("harmless log line with no credentials at all\n", 80_000) // ~3.5 MiB
	body := filler + "aws_key=" + "AKIA" + "ABCDEFGHIJKLMNOP" + "\n"
	if err := os.WriteFile(filepath.Join(root, "server.log"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, Options{SecretScan: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := kinds(r)["aws_access_key"]; !ok {
		t.Fatalf("a credential past the old 2 MiB cutoff was missed: %#v", r.Findings)
	}
}

// TestSecretStraddlingAChunkBoundaryIsFound proves the streaming window keeps
// enough overlap that chunking cannot hide a credential.
func TestSecretStraddlingAChunkBoundaryIsFound(t *testing.T) {
	secret := "github_" + "pat_" + strings.Repeat("m4Nb", 8)
	// Land the value across the end of the first window. The filler must end on
	// a non-word byte, or the token's leading word boundary never matches and
	// the test would fail for a reason that has nothing to do with chunking.
	start := contentChunk + contentOverlap - len(secret)/2
	prefix := strings.Repeat("x", start-1) + "\n"

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(prefix+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, Options{SecretScan: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := kinds(r)["github_fine_grained_token"]; !ok {
		t.Fatalf("a credential spanning a chunk boundary was missed: %#v", r.Findings)
	}
}

func TestBinaryFilesAreNotContentScanned(t *testing.T) {
	body := "\x00\x01\x02binary blob " + "AKIA" + "ABCDEFGHIJKLMNOP"
	r := scanOne(t, "asset.bin", body)
	if len(r.Findings) != 0 {
		t.Fatalf("binary content should not produce findings: %#v", r.Findings)
	}
}

func TestEveryRuleDeclaresAConfidence(t *testing.T) {
	for _, rule := range contentRules {
		if rule.confidence != ConfidenceHigh && rule.confidence != ConfidenceMedium {
			t.Fatalf("rule %q has confidence %q", rule.kind, rule.confidence)
		}
		if rule.kind == "" || rule.re == nil {
			t.Fatalf("malformed rule %#v", rule)
		}
	}
}
