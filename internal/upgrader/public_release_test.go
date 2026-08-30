package upgrader

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func httpResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestPublicReleaseClientUsesAnonymousHTTP(t *testing.T) {
	var sawAuth bool
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		switch {
		case r.URL.Host == "api.github.com" && strings.HasSuffix(r.URL.Path, "/releases/latest"):
			return httpResponse(200, `{"tag_name":"v1.2.2","assets":[]}`), nil
		case r.URL.Host == "api.github.com" && strings.Contains(r.URL.Path, "/releases/tags/"):
			return httpResponse(200, `{"tag_name":"v1.2.2","assets":[{"name":"GitMake_v1.2.2_SHA256.txt","browser_download_url":"https://github.com/stickleetoto/GitMake/releases/download/v1.2.2/GitMake_v1.2.2_SHA256.txt"}]}`), nil
		case r.URL.Host == "github.com":
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString("checksum-data"))}, nil
		default:
			return httpResponse(404, "not found"), nil
		}
	})}
	c := PublicReleaseClient{HTTP: client, APIBase: defaultGitHubAPIBase}

	tag, err := c.LatestReleaseTag(releaseRepo)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v1.2.2" {
		t.Fatalf("tag=%q", tag)
	}
	d := t.TempDir()
	p, err := c.DownloadReleaseAsset(releaseRepo, tag, "GitMake_v1.2.2_SHA256.txt", d)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "checksum-data" {
		t.Fatalf("unexpected download %q", string(b))
	}
	if filepath.Base(p) != "GitMake_v1.2.2_SHA256.txt" {
		t.Fatalf("unexpected path %q", p)
	}
	if sawAuth {
		t.Fatal("public updater unexpectedly sent Authorization header")
	}
}

func TestValidateDownloadURL(t *testing.T) {
	good := []string{
		"https://github.com/stickleetoto/GitMake/releases/download/v1.2.2/file.zip",
		"https://objects.githubusercontent.com/example",
		"https://release-assets.githubusercontent.com/example",
	}
	for _, raw := range good {
		if err := validateDownloadURL(raw); err != nil {
			t.Fatalf("expected allowed URL %s: %v", raw, err)
		}
	}
	bad := []string{
		"http://github.com/stickleetoto/GitMake/releases/download/v1.2.2/file.zip",
		"https://example.com/file.zip",
	}
	for _, raw := range bad {
		if err := validateDownloadURL(raw); err == nil {
			t.Fatalf("expected rejected URL %s", raw)
		}
	}
}
