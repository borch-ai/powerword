//go:build !windows

package loop

import (
	"os"

	"golang.org/x/sys/unix"
)

func getTerminalWidth() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err == nil && ws.Col > 0 {
		return int(ws.Col)
	}
	return 80
}
