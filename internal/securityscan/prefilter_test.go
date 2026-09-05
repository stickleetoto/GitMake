package securityscan

import (
	"bytes"
	"strings"
	"testing"
)

// The literal gate in findMatch is a speed optimisation applied to the security
// gate itself, which is the most dangerous place in GitMake to make one. A
// literal that is too narrow does not fail loudly; it just stops detecting a
// credential, and every existing test stays green while the scanner goes blind.
//
// These tests are what make the optimisation safe to keep. Every rule is run
// with the gate and with the gate bypassed, over samples covering every branch
// of every alternation, and the two must agree exactly.

// findMatchUngated is findMatch with the literal gate removed. It is the
// reference implementation that the gated one is held against.
func findMatchUngated(rule contentRule, window []byte) int {
	bare := rule
	bare.literals = nil
	return findMatch(bare, window)
}

// Samples are assembled from fragments so this file never contains a literal
// credential signature; a real one here would block GitMake's own publish.
// TestScannerTestFixturesDoNotContainLiteralSecretSignatures enforces that.
func credentialSamples() map[string][]string {
	long := func(seed string, n int) string { return strings.Repeat(seed, n) }
	begin := "-----" + "BEGIN "
	end := " KEY-----"
	return map[string][]string{
		"private_key": {
			begin + "RSA PRIVATE" + end,
			begin + "PRIVATE" + end,
			begin + "EC PRIVATE" + end,
			begin + "DSA PRIVATE" + end,
			begin + "OPENSSH PRIVATE" + end,
			begin + "ENCRYPTED PRIVATE" + end,
			begin + "PGP PRIVATE" + " KEY BLOCK-----",
		},
		"github_token": {
			"gh" + "p_" + long("A1b2", 9),
			"gh" + "o_" + long("A1b2", 9),
			"gh" + "u_" + long("A1b2", 9),
			"gh" + "s_" + long("A1b2", 9),
			"gh" + "r_" + long("A1b2", 9),
		},
		"github_fine_grained_token": {"github" + "_pat_" + long("A1b2", 9)},
		"aws_access_key": {
			"AK" + "IA" + long("ABCD", 4),
			"AS" + "IA" + long("WXYZ", 4),
		},
		"slack_token": {
			"xo" + "xb-" + long("a1B2", 4),
			"xo" + "xp-" + long("a1B2", 4),
			"xo" + "xa-" + long("a1B2", 4),
			"xo" + "xr-" + long("a1B2", 4),
			"xo" + "xs-" + long("a1B2", 4),
		},
		"slack_webhook": {"https://hooks" + ".slack.com/services/" + long("T0aB", 6)},
		"discord_webhook": {
			"https://discord" + ".com/api/webhooks/1234567890123/" + long("nQ7x", 6),
			"https://canary.discord" + ".com/api/webhooks/1234567890123/" + long("nQ7x", 6),
			"https://ptb.discord" + ".com/api/webhooks/1234567890123/" + long("nQ7x", 6),
			"https://discordapp" + ".com/api/webhooks/1234567890123/" + long("nQ7x", 6),
		},
		"anthropic_api_key": {"sk-" + "ant-api03-" + long("A1b2", 8)},
		"openai_api_key": {
			"sk-" + long("Xy9z", 12),
			"sk-" + "proj-" + long("Xy9z", 12),
			"sk-" + "svcacct-" + long("Xy9z", 12),
			"sk-" + "admin-" + long("Xy9z", 12),
		},
		"huggingface_token":          {"hf" + "_" + long("aBcD", 9)},
		"google_api_key":             {"AI" + "za" + long("Sy0b", 8) + "abc"},
		"google_oauth_client_secret": {"GOC" + "SPX-" + long("q7Wq", 6)},
		"gcp_service_account":        {"\"type\": \"service" + "_account\""},
		"stripe_secret_key": {
			"sk" + "_live_" + long("Zt4R", 6),
			"rk" + "_live_" + long("Zt4R", 6),
		},
		"sendgrid_api_key":  {"SG" + "." + long("aB1c", 6) + "." + long("dE2f", 6)},
		"npm_token":         {"npm" + "_" + long("k3Mn", 9)},
		"azure_storage_key": {"Account" + "Key=" + long("bXk9", 16) + "=="},
		"connection_string_password": {
			"postgres:" + "//admin:hunter2@db.internal:5432/app",
			"mongodb:" + "//svc:9f3xQ2@cluster0.example.net/db",
		},
		"jwt": {"ey" + "J" + long("aBcD", 4) + ".ey" + "J" + long("eFgH", 4) + "." + long("iJkL", 4)},
	}
}

func ruleNamed(t *testing.T, kind string) contentRule {
	t.Helper()
	for _, r := range contentRules {
		if r.kind == kind {
			return r
		}
	}
	t.Fatalf("no rule named %s", kind)
	return contentRule{}
}

func TestEveryRuleDeclaresALiteralGate(t *testing.T) {
	for _, rule := range contentRules {
		if len(rule.literals) == 0 {
			t.Errorf("rule %s declares no literal gate; it runs its regex over every byte of every file", rule.kind)
		}
		for _, lit := range rule.literals {
			if len(lit) < 3 {
				t.Errorf("rule %s has literal %q; a gate shorter than 3 bytes barely filters anything", rule.kind, lit)
			}
		}
	}
}

// TestEveryRuleHasASample keeps the sample table from drifting behind the rules
// it protects. A rule with no sample is a rule whose gate nothing checks.
func TestEveryRuleHasASample(t *testing.T) {
	samples := credentialSamples()
	for _, rule := range contentRules {
		if len(samples[rule.kind]) == 0 {
			t.Errorf("rule %s has no sample; add one so its literal gate is proven", rule.kind)
		}
	}
	for kind := range samples {
		found := false
		for _, rule := range contentRules {
			if rule.kind == kind {
				found = true
			}
		}
		if !found {
			t.Errorf("sample provided for unknown rule %s", kind)
		}
	}
}

// TestSamplesAreActuallyDetected proves the samples are real positives. Without
// it the agreement test below could pass trivially on inputs matching nothing.
func TestSamplesAreActuallyDetected(t *testing.T) {
	for kind, samples := range credentialSamples() {
		rule := ruleNamed(t, kind)
		for _, s := range samples {
			body := []byte("value = \"" + s + "\"\n")
			if findMatchUngated(rule, body) < 0 {
				t.Errorf("%s: sample %q does not match its own rule", kind, s)
			}
			if findMatch(rule, body) < 0 {
				t.Errorf("%s: the literal gate suppressed its own sample %q", kind, s)
			}
		}
	}
}

// TestLiteralGatesNeverChangeAVerdict is the invariant. Every rule sees every
// sample -- its own and every other rule's -- gated and ungated, and both must
// return the identical offset. A literal too narrow for some branch of a
// pattern shows up here as a rule finding nothing where the reference finds
// something.
func TestLiteralGatesNeverChangeAVerdict(t *testing.T) {
	var all []string
	var corpus []byte
	for _, samples := range credentialSamples() {
		for _, s := range samples {
			all = append(all, s)
			corpus = append(corpus, []byte("cfg = \""+s+"\"\n")...)
		}
	}

	for _, rule := range contentRules {
		t.Run(rule.kind, func(t *testing.T) {
			for _, s := range all {
				bodies := [][]byte{
					[]byte("value = \"" + s + "\"\n"),
					[]byte(s),
					[]byte("prefix\n" + s + "\nsuffix\n"),
					append(bytes.Repeat([]byte("an harmless line of code\n"), 200), []byte(" "+s+" ")...),
				}
				for _, body := range bodies {
					got, want := findMatch(rule, body), findMatchUngated(rule, body)
					if got != want {
						t.Fatalf("gate changed the verdict: got %d, want %d, for sample %q", got, want, s)
					}
				}
			}
			// And the whole corpus at once, which is what a real chunk looks like.
			if got, want := findMatch(rule, corpus), findMatchUngated(rule, corpus); got != want {
				t.Fatalf("gate changed the verdict on the full corpus: got %d, want %d", got, want)
			}
		})
	}
}

// TestGateAgreesOnOrdinarySource covers the other direction: text that must
// stay clean has to stay clean, gate or no gate.
func TestGateAgreesOnOrdinarySource(t *testing.T) {
	clean := []string{
		"package main\n\nfunc main() { println(\"hello\") }\n",
		"const highlight = true // gh, sk, xo, AI, SG are ordinary here\n",
		"see https://example.com/docs and https://api.example.com/v1/users\n",
		"night light bright fight sight might right tight\n",
		strings.Repeat("the quick brown fox jumps over the lazy dog\n", 500),
	}
	for _, rule := range contentRules {
		for _, body := range clean {
			got, want := findMatch(rule, []byte(body)), findMatchUngated(rule, []byte(body))
			if got != want {
				t.Fatalf("%s: gate changed the verdict on clean text: got %d, want %d", rule.kind, got, want)
			}
		}
	}
}
