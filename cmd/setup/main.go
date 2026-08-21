package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gitmake/internal/aiconnect"
	"gitmake/internal/installer"
)

func main() {
	fmt.Println("GitMake Setup 0.7.2")
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
	fmt.Println("✓ Installed", target)
	if added {
		fmt.Println("✓ Added GitMake to your user PATH")
	} else {
		fmt.Println("✓ GitMake is already on your user PATH")
	}

	if _, err := exec.LookPath("claude"); err == nil {
		mgr := aiconnect.Manager{GitMakePath: target}
		status, changed, setupErr := mgr.Setup(false)
		if setupErr != nil {
			fmt.Println("! Claude Code detected, but MCP setup needs attention:")
			fmt.Println("  " + setupErr.Error())
			fmt.Println("  You can retry later with: gitmake ai setup")
		} else {
			if changed {
				fmt.Println("✓ Connected Claude Code MCP (read-only)")
			} else {
				fmt.Println("✓ Claude Code MCP already connected")
			}
			fmt.Println("  Access:", status.Access)
		}
	} else {
		fmt.Println("· Claude Code not detected — automatic AI connection skipped")
		fmt.Println("  Claude:  gitmake ai setup")
		fmt.Println("  Generic: gitmake ai setup --client generic --json")
	}

	fmt.Println()
	if added {
		fmt.Println("Open a new PowerShell/Terminal window, then run:")
		fmt.Println("  gitmake doctor")
		fmt.Println("  gitmake ai status")
	} else {
		fmt.Println("Ready. Try:")
		fmt.Println("  gitmake doctor")
		fmt.Println("  gitmake ai status")
	}
	fmt.Println()
	fmt.Print("Press Enter to close Setup...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func fail(err error) {
	fmt.Println("× Setup failed:", err)
	fmt.Println()
	fmt.Print("Press Enter to close Setup...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}
