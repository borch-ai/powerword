package loop

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var ErrSessionPaused = errors.New("session paused by user")

// SecurityProfile defines the level of permission restriction for tool calls.
type SecurityProfile int

const (
	// Interactive requires terminal prompts for modifying actions.
	Interactive SecurityProfile = iota
	// ReadOnly allows reads, blocks modifications/scripts.
	ReadOnly
	// Bypass trusts all actions without prompting.
	Bypass
)

// Guard provides authorization checks for tool executions.
type Guard struct {
	Profile SecurityProfile
	In      io.Reader
	Out     io.Writer
}

// NewGuard creates a new Guard with the specified profile.
func NewGuard(profile SecurityProfile, in io.Reader, out io.Writer) *Guard {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}
	return &Guard{
		Profile: profile,
		In:      in,
		Out:     out,
	}
}

// Authorize determines if a tool call should be executed based on the security profile.
// If the profile is Interactive, it blocks and prompts the user on the terminal.
func (g *Guard) Authorize(toolName string, args map[string]interface{}) (bool, error) {
	if g.Profile == Bypass {
		return true, nil
	}

	isMutating := false
	mutatingKeywords := []string{"write", "delete", "create", "run", "execute", "modify", "update", "push", "commit", "remove", "bash", "cmd", "shell", "replace"}
	lowerName := strings.ToLower(toolName)
	for _, kw := range mutatingKeywords {
		if strings.Contains(lowerName, kw) {
			isMutating = true
			break
		}
	}

	if g.Profile == ReadOnly {
		if isMutating {
			return false, fmt.Errorf("tool '%s' blocked by ReadOnly security profile", toolName)
		}
		return true, nil
	}

	// Interactive Profile
	if !isMutating {
		return true, nil
	}

	_, _ = fmt.Fprintf(g.Out, "\n\x1b[1;33m[?]\x1b[0m Allow tool \x1b[1;36m%s\x1b[0m? (y/N/p to pause) ", toolName)

	reader := bufio.NewReader(g.In)
	resp, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("failed to read from terminal: %w", err)
	}

	resp = strings.TrimSpace(strings.ToLower(resp))
	if resp == "p" || resp == "pause" {
		return false, ErrSessionPaused
	}
	if resp == "" && err == io.EOF {
		return false, nil
	}
	if resp == "y" || resp == "yes" {
		return true, nil
	}

	return false, nil
}
