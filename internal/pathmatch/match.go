package pathmatch

import (
	"path"
	"strings"
)

// Match reports whether a repository-relative slash path matches a small,
// predictable glob language. Standard path.Match syntax is supported and a
// trailing /** means "this directory and everything below it".
func Match(pattern, name string) bool {
	pattern = normalize(pattern)
	name = normalize(name)
	if pattern == "" || name == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return name == prefix || strings.HasPrefix(name, prefix+"/")
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

func Any(patterns []string, name string) bool {
	for _, p := range patterns {
		if Match(p, name) {
			return true
		}
	}
	return false
}

func normalize(v string) string {
	v = strings.ReplaceAll(strings.TrimSpace(v), "\\", "/")
	v = strings.TrimPrefix(v, "./")
	v = strings.Trim(v, "/")
	return v
}
