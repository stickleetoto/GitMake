package upgrader

import (
	"net/http"
	"strings"
	"testing"
)

func statusResponse(code int, header map[string]string) *http.Response {
	resp := &http.Response{StatusCode: code, Header: make(http.Header)}
	for k, v := range header {
		resp.Header.Set(k, v)
	}
	return resp
}

func TestDescribeHTTPFailureIsActionable(t *testing.T) {
	cases := []struct {
		name   string
		resp   *http.Response
		expect string
	}{
		{
			name:   "anonymous rate limit",
			resp:   statusResponse(403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1735689600"}),
			expect: "rate limit reached",
		},
		{
			name:   "blocked rather than limited",
			resp:   statusResponse(403, nil),
			expect: "proxy or network policy",
		},
		{
			name:   "too many requests",
			resp:   statusResponse(429, map[string]string{"X-RateLimit-Remaining": "0"}),
			expect: "rate limit reached",
		},
		{
			name:   "missing release",
			resp:   statusResponse(404, nil),
			expect: "no matching published release",
		},
		{
			name:   "github outage",
			resp:   statusResponse(503, nil),
			expect: "GitHub is currently unavailable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := describeHTTPFailure("check latest GitMake release", tc.resp)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.expect) {
				t.Fatalf("error %q does not explain %q", err.Error(), tc.expect)
			}
		})
	}
}
