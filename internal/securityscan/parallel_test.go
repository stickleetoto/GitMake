package securityscan

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Content scanning runs across several goroutines. A security gate that
// reports different findings depending on which worker finished first would be
// worse than a slow one, so these tests hold the parallel scan against the
// sequential scan of the same tree and require the reports to be identical --
// findings, order, counts and all.

// withWorkers runs fn with a fixed worker count.
func withWorkers(t *testing.T, n int, fn func()) {
	t.Helper()
	saved := scanWorkers
	scanWorkers = n
	defer func() { scanWorkers = saved }()
	fn()
}

// mixedTree writes a tree that exercises every path through the scanner:
// clean files, several kinds of secret, more than one kind in a single file, a
// secret-by-name file, a large file, and nested directories.
func mixedTree(t *testing.T, files int) string {
	t.Helper()
	root := t.TempDir()
	samples := credentialSamples()
	kinds := make([]string, 0, len(samples))
	for k := range samples {
		kinds = append(kinds, k)
	}
	// Deterministic order, since map iteration is not.
	for i := 0; i < len(kinds); i++ {
		for j := i + 1; j < len(kinds); j++ {
			if kinds[j] < kinds[i] {
				kinds[i], kinds[j] = kinds[j], kinds[i]
			}
		}
	}

	clean := strings.Repeat("package main // an ordinary line of code\n", 40)
	for i := 0; i < files; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%02d", i%7))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := clean
		switch i % 4 {
		case 1:
			// One secret.
			k := kinds[i%len(kinds)]
			body += "token = \"" + samples[k][0] + "\"\n"
		case 2:
			// Two different kinds in one file.
			a := kinds[i%len(kinds)]
			b := kinds[(i+3)%len(kinds)]
			body += "a = \"" + samples[a][0] + "\"\nfiller\nb = \"" + samples[b][0] + "\"\n"
		case 3:
			// Long file, secret near the end.
			k := kinds[(i+5)%len(kinds)]
			body += strings.Repeat("padding line that matches nothing\n", 3000)
			body += "late = \"" + samples[k][0] + "\"\n"
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.go", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A file that is a secret by name rather than by contents.
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DEBUG=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A large file, to exercise the LargeFiles path alongside the findings.
	if err := os.WriteFile(filepath.Join(root, "big.bin"), []byte(strings.Repeat("x", 3<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func mixedOptions() Options {
	return Options{SecretScan: true, WarnFileBytes: 1 << 20, MaxGitFileBytes: 64 << 20}
}

func TestParallelScanMatchesSequentialScan(t *testing.T) {
	root := mixedTree(t, 60)

	var sequential Report
	withWorkers(t, 1, func() {
		r, err := Scan(root, mixedOptions())
		if err != nil {
			t.Fatal(err)
		}
		sequential = r
	})

	if len(sequential.Findings) < 20 {
		t.Fatalf("the fixture must produce plenty of findings for this to mean anything, got %d", len(sequential.Findings))
	}
	if !sequential.Blocking {
		t.Fatal("the fixture must block")
	}

	for _, workers := range []int{2, 3, 4, 8, 16} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			withWorkers(t, workers, func() {
				// Repeated, because a scheduling-dependent difference does not
				// necessarily show up on the first run.
				for i := 0; i < 20; i++ {
					got, err := Scan(root, mixedOptions())
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, sequential) {
						t.Fatalf("run %d with %d workers differs from the sequential scan:\n parallel: %+v\n sequential: %+v",
							i, workers, got, sequential)
					}
				}
			})
		})
	}
}

// TestParallelScanIsDeterministicAcrossRuns pins the ordering promise on its
// own: the same tree scanned repeatedly at the default worker count must give
// the identical report every time.
func TestParallelScanIsDeterministicAcrossRuns(t *testing.T) {
	root := mixedTree(t, 40)
	first, err := Scan(root, mixedOptions())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		got, err := Scan(root, mixedOptions())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs from the first scan of the same tree", i)
		}
	}
}

// TestScanCountsEveryFileOnce guards the counter the walk keeps while the scan
// itself is parallel.
func TestScanCountsEveryFileOnce(t *testing.T) {
	root := mixedTree(t, 25)
	want := 0
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			want++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Scan(root, mixedOptions())
	if err != nil {
		t.Fatal(err)
	}
	if r.ScannedFiles != want {
		t.Fatalf("ScannedFiles = %d, want %d", r.ScannedFiles, want)
	}
}

// TestUnreadableFileFailsTheSameWayEveryTime covers the error path. A file the
// scanner cannot read must produce the same error regardless of which worker
// happened to reach it, because a security scan that sometimes fails and
// sometimes passes is the worst outcome available.
func TestUnreadableFileFailsTheSameWayEveryTime(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 12; i++ {
		body := "package main // ordinary\n"
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%02d.go", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory entry that Stat reports as a regular file cannot be forged
	// portably, so remove a file after the walk instead: open() then fails.
	// Simulate by pointing at a path that vanishes -- a target whose file is
	// deleted between the walk and the scan.
	missing := filepath.Join(root, "f05.go")

	var last string
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(missing, []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		targets := []scanTarget{}
		for j := 0; j < 12; j++ {
			p := filepath.Join(root, fmt.Sprintf("f%02d.go", j))
			targets = append(targets, scanTarget{path: p, rel: filepath.Base(p)})
		}
		if err := os.Remove(missing); err != nil {
			t.Fatal(err)
		}
		_, errs := scanAll(targets)
		var got string
		for _, e := range errs {
			if e != nil {
				got = e.Error()
				break
			}
		}
		if got == "" {
			t.Fatal("removing a target should surface a read error")
		}
		if last != "" && got != last {
			t.Fatalf("error differs between runs: %q then %q", last, got)
		}
		last = got
	}
}
