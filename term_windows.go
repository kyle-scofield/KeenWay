//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableTerminalColors turns on ANSI colours in the Windows console (Windows 10+).
func enableTerminalColors() {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
	windows.SetConsoleOutputCP(65001)
}
