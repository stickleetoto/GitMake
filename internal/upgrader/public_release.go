package upgrader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultGitHubAPIBase = "https://api.github.com"

type PublicReleaseClient struct {
	HTTP    *http.Client
	APIBase string
}

type publicRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func NewPublicReleaseClient() PublicReleaseClient {
	return PublicReleaseClient{
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		APIBase: defaultGitHubAPIBase,
	}
}

func (c PublicReleaseClient) LatestReleaseTag(target string) (string, error) {
	rel, err := c.latestRelease(target)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return "", fmt.Errorf("GitHub returned an empty latest release tag")
	}
	return rel.TagName, nil
}

func (c PublicReleaseClient) DownloadReleaseAsset(target, tag, asset, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rel, err := c.releaseByTag(target, tag)
	if err != nil {
		return "", err
	}
	downloadURL := ""
	for _, a := range rel.Assets {
		if a.Name == asset {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("release %s does not contain asset %s", tag, asset)
	}
	if err := validateDownloadURL(downloadURL); err != nil {
		return "", fmt.Errorf("unsafe release asset URL: %w", err)
	}

	client := c.httpClient()
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	setPublicHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download GitMake release asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return "", describeHTTPFailure("download GitMake release asset", resp)
	}

	path := filepath.Join(dir, filepath.Base(asset))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	return path, nil
}

func (c PublicReleaseClient) latestRelease(target string) (publicRelease, error) {
	return c.fetchRelease("/repos/" + target + "/releases/latest")
}

func (c PublicReleaseClient) releaseByTag(target, tag string) (publicRelease, error) {
	return c.fetchRelease("/repos/" + target + "/releases/tags/" + url.PathEscape(tag))
}

func (c PublicReleaseClient) fetchRelease(path string) (publicRelease, error) {
	var out publicRelease
	base := strings.TrimRight(c.APIBase, "/")
	if base == "" {
		base = defaultGitHubAPIBase
	}
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return out, err
	}
	setPublicHeaders(req)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return out, fmt.Errorf("check latest GitMake release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return out, describeHTTPFailure("check latest GitMake release", resp)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := dec.Decode(&out); err != nil {
		return out, fmt.Errorf("parse GitHub release response: %w", err)
	}
	return out, nil
}

func (c PublicReleaseClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func setPublicHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "GitMake-Updater")
}

// describeHTTPFailure turns a GitHub status code into something a user can act
// on. "GitHub returned HTTP 403" gives no clue that the fix is usually just
// waiting out an anonymous rate limit.
func describeHTTPFailure(what string, resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusTooManyRequests:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			reset := "shortly"
			if raw := resp.Header.Get("X-RateLimit-Reset"); raw != "" {
				if secs, err := strconv.ParseInt(raw, 10, 64); err == nil {
					reset = time.Unix(secs, 0).Local().Format("15:04:05")
				}
			}
			return fmt.Errorf("%s: GitHub rate limit reached for anonymous requests; try again after %s", what, reset)
		}
		return fmt.Errorf("%s: GitHub refused the request (HTTP %d). A proxy or network policy may be blocking api.github.com", what, resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("%s: no matching published release or asset was found for %s", what, releaseRepo)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%s: GitHub is currently unavailable (HTTP %d); try again later", what, resp.StatusCode)
	}
	return fmt.Errorf("%s: GitHub returned HTTP %d", what, resp.StatusCode)
}

func validateDownloadURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return fmt.Errorf("download URL must use HTTPS")
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "objects.githubusercontent.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return fmt.Errorf("unexpected download host %q", host)
	}
	return nil
}
