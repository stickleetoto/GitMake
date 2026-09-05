package securityscan

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The secret scan reads every byte of every file being published, so its
// throughput is the floor on how fast a publish can be. These benchmarks exist
// so a change to the rule table cannot quietly make that floor worse.
//
// Each tree is warmed before the timer starts: on Windows the first read of a
// freshly written file also pays for the virus scanner, which would otherwise
// swamp the number being measured.

func benchTree(tb testing.TB, dirs, perDir, size int) string {
	tb.Helper()
	root := tb.TempDir()
	line := []byte("package main // an ordinary line of source code, no secrets at all\n")
	body := make([]byte, 0, size+len(line))
	for len(body) < size {
		body = append(body, line...)
	}
	for d := 0; d < dirs; d++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%02d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		for f := 0; f < perDir; f++ {
			p := filepath.Join(dir, fmt.Sprintf("file%03d.go", f))
			if err := os.WriteFile(p, body, 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}
	return root
}

func benchOptions() Options {
	return Options{SecretScan: true, WarnFileBytes: 10 << 20, MaxGitFileBytes: 100 << 20}
}

func BenchmarkScanTree(b *testing.B) {
	cases := []struct {
		name               string
		dirs, perDir, size int
	}{
		{"100files_4KiB", 10, 10, 4 << 10},
		{"500files_16KiB", 25, 20, 16 << 10},
		{"2000files_16KiB", 40, 50, 16 << 10},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			root := benchTree(b, tc.dirs, tc.perDir, tc.size)
			b.SetBytes(int64(tc.dirs * tc.perDir * tc.size))
			// Warm the page cache and the virus scanner.
			if _, err := Scan(root, benchOptions()); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r, err := Scan(root, benchOptions())
				if err != nil {
					b.Fatal(err)
				}
				if r.Blocking {
					b.Fatal("the synthetic tree must be clean")
				}
			}
		})
	}
}

// BenchmarkScanContentOneFile isolates the per-byte rule cost from the walk and
// from the filesystem.
func BenchmarkScanContentOneFile(b *testing.B) {
	root := benchTree(b, 1, 1, 4<<20)
	p := filepath.Join(root, "pkg00", "file000.go")
	st, err := os.Stat(p)
	if err != nil {
		b.Fatal(err)
	}
	buf := newScanWindow()
	if _, err := scanContent(p, buf); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(st.Size())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := scanContent(p, buf); err != nil {
			b.Fatal(err)
		}
	}
}
