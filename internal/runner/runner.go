package runner

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Result struct {
	Stdout string
	Stderr string
	Code   int
}

type Runner struct {
	Verbose bool
}

func (r Runner) Run(dir, name string, args ...string) (Result, error) {
	if r.Verbose {
		fmt.Printf("$ %s %s\n", name, joinForDisplay(args))
	}
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := Result{Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String()), Code: 0}
	if err == nil {
		return res, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.Code = ee.ExitCode()
		return res, nil
	}
	return res, err
}

func joinForDisplay(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\n\"") {
			out = append(out, fmt.Sprintf("%q", a))
		} else {
			out = append(out, a)
		}
	}
	return strings.Join(out, " ")
}
