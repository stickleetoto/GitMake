package main

import (
	"bufio"
	"fmt"
	"gitmake/internal/installer"
	"os"
	"path/filepath"
)

func main() {
	fmt.Println("GitMake Setup 0.5.2")
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
		fmt.Println("  Open a new PowerShell/Terminal window, then run: gitmake doctor")
	} else {
		fmt.Println("✓ GitMake is already on your user PATH")
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
