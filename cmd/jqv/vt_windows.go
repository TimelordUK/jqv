package main

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// enableVT turns on escape sequences in the Windows console, for :cls. Windows Terminal has
// them on already; the old console host needs asking.
func enableVT(w io.Writer) {
	f, ok := w.(*os.File)
	if !ok {
		return
	}
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
