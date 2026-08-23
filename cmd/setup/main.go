package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitmake/internal/aiconnect"
	"gitmake/internal/installer"
)

func main() {
	fmt.Println("GitMake Setup 1.0.0")
	fmt.Println()
	exe, err := os.Executable()
	if err != nil {
		fail(err)
	}
	sibling := filepath.Join(filepath.Dir(exe), "gitmake.exe")
	if _, err := os.Stat(sibling); err != nil {
		fail(fmt.Errorf("gitmake.exe must be beside GitMake-Setup.exe"))
	}
	target, added, err := installer.InstallSibling(sibling)
	if err != nil {
		fail(err)
	}

	fmt.Println("Install")
	fmt.Println("✓ GitMake CLI       " + target)
	if added {
		fmt.Println("✓ User PATH         added (new terminals will pick it up)")
	} else {
		fmt.Println("✓ User PATH         already configured")
	}

	fmt.Println("\nReadiness")
	gitOK, gitDetail := commandVersion("git", "--version")
	printCheck("Git", gitOK, gitDetail)
	ghOK, ghDetail := commandVersion("gh", "--version")
	printCheck("GitHub CLI", ghOK, ghDetail)

	authOK := false
	authDetail := "not checked"
	if ghOK {
		res := exec.Command("gh", "auth", "status", "--hostname", "github.com")
		out, authErr := res.CombinedOutput()
		authOK = authErr == nil
		if authOK {
			authDetail = "signed in"
			if u, e := exec.Command("gh", "api", "user", "--jq", ".login").Output(); e == nil && strings.TrimSpace(string(u)) != "" {
				authDetail = strings.TrimSpace(string(u))
			}
		} else {
			authDetail = firstLine(string(out))
			if authDetail == "" {
				authDetail = "run gh auth login"
			}
		}
	}
	printCheck("GitHub login", authOK, authDetail)

	claudeOK := false
	mcpOK := false
	if _, err := exec.LookPath("claude"); err == nil {
		claudeOK = true
		verOK, ver := commandVersion("claude", "--version")
		if !verOK || ver == "" {
			ver = "detected"
		}
		printCheck("Claude Code", true, ver)
		mgr := aiconnect.Manager{GitMakePath: target}
		status, changed, setupErr := mgr.Setup(false)
		if setupErr != nil {
			printCheck("Claude MCP", false, setupErr.Error())
		} else {
			mcpOK = true
			detail := status.Access
			if changed {
				detail += " · connected"
			} else {
				detail += " · already connected"
			}
			printCheck("Claude MCP", true, detail)
		}
	} else {
		fmt.Println("· Claude Code       not detected (optional)")
		fmt.Println("· Claude MCP        skipped")
	}

	fmt.Println()
	if gitOK && ghOK && authOK {
		fmt.Println("✓ Ready")
		fmt.Println("  Open a project folder and run: gitmake")
		if claudeOK && mcpOK {
			fmt.Println("  Or ask Claude: \"이 프로젝트 GitHub에 올려줘\"")
		}
	} else {
		fmt.Println("Needs attention")
		if !gitOK {
			fmt.Println("  → Install Git")
		}
		if !ghOK {
			fmt.Println("  → Install GitHub CLI (gh)")
		} else if !authOK {
			fmt.Println("  → Run: gh auth login")
		}
		fmt.Println("  → Then run: gitmake doctor")
	}

	fmt.Println()
	fmt.Print("Press Enter to close Setup...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func commandVersion(name string, args ...string) (bool, string) {
	p, err := exec.LookPath(name)
	if err != nil {
		return false, "not found"
	}
	out, err := exec.Command(p, args...).CombinedOutput()
	detail := firstLine(string(out))
	if err != nil {
		if detail == "" {
			detail = err.Error()
		}
		return false, detail
	}
	if detail == "" {
		detail = "detected"
	}
	return true, detail
}

func printCheck(label string, ok bool, detail string) {
	mark := "✓"
	if !ok {
		mark = "×"
	}
	fmt.Printf("%s %-17s %s\n", mark, label, detail)
}

func firstLine(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func fail(err error) {
	fmt.Println("× Setup failed:", err)
	fmt.Println()
	fmt.Print("Press Enter to close Setup...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}
