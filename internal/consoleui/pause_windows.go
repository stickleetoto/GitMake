//go:build windows

package consoleui

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	getConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// LaunchedStandalone reports whether GitMake appears to own a console that was
// created just for this process (the usual case when an .exe is double-clicked
// in Explorer). In an existing cmd/PowerShell/Terminal console there are
// normally two or more attached processes.
func LaunchedStandalone() bool {
	pids := make([]uint32, 8)
	n, _, _ := getConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)),
	)
	return n == 1
}

// Pause waits so a double-clicked console does not disappear before the user
// can read success or error output.
func Pause() {
	fmt.Println()
	fmt.Print("Press Enter to close GitMake...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
