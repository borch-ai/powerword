//go:build windows

package mcp

import (
	"os"
)

var shutdownSignals = []os.Signal{os.Interrupt}
