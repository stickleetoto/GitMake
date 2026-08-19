package main

import (
	"gitmake/internal/app"
	"gitmake/internal/consoleui"
	"os"
	"path/filepath"
)

func main() {
	standalone := consoleui.LaunchedStandalone()

	// Explorer launches do not always inherit the executable's directory as
	// the current working directory. For the double-click workflow, make the
	// folder containing gitmake.exe the working directory so gitmake.json and
	// the ZIP beside it are found reliably.
	if standalone && len(os.Args) == 1 {
		if exe, err := os.Executable(); err == nil {
			_ = os.Chdir(filepath.Dir(exe))
		}
	}

	code := app.Main(os.Args[1:])
	if standalone {
		consoleui.Pause()
	}
	os.Exit(code)
}
