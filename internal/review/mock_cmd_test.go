package review

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type mockCmd struct {
	runFunc            func() error
	outputFunc         func() ([]byte, error)
	combinedOutputFunc func() ([]byte, error)
	stdout             io.Writer
	stderr             io.Writer
	dir                string
}

func (m *mockCmd) Run() error {
	if m.runFunc != nil {
		return m.runFunc()
	}
	return nil
}

func (m *mockCmd) Output() ([]byte, error) {
	if m.outputFunc != nil {
		return m.outputFunc()
	}
	return []byte("success"), nil
}

func (m *mockCmd) CombinedOutput() ([]byte, error) {
	if m.combinedOutputFunc != nil {
		return m.combinedOutputFunc()
	}
	return []byte("success"), nil
}

func (m *mockCmd) SetStdout(w io.Writer) { m.stdout = w }
func (m *mockCmd) SetStderr(w io.Writer) { m.stderr = w }
func (m *mockCmd) SetDir(dir string)     { m.dir = dir }

func mockReviewExec(command string, args []string) Cmd {
	return &mockCmd{
		runFunc: func() error {
			if command == "fail" {
				return fmt.Errorf("command failed")
			}
			return nil
		},
		outputFunc: func() ([]byte, error) {
			if command == "gh" {
				if len(args) > 0 && args[0] == "gh_invalid" {
					return []byte("invalid json"), nil
				}
				return []byte(`{"body": "### Goal\ntest\n### Proposed Changes\nchanges\n### Verification Plan\nverif"}`), nil
			}
			if command == "fail" {
				return nil, fmt.Errorf("command failed")
			}
			return []byte("success"), nil
		},
		combinedOutputFunc: func() ([]byte, error) {
			if command == "make" {
				if len(args) > 0 && args[0] == "fail" {
					return []byte("mock make failed"), fmt.Errorf("make error")
				}
				return []byte("mock make all success"), nil
			}
			if command == "fail" {
				return nil, fmt.Errorf("command failed")
			}
			return []byte("success"), nil
		},
	}
}

// mockOSCmd spawns the TestHelperProcess using the currently executing test binary (os.Args[0])
func mockOSCmd(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	//nolint:gosec // G204: test helper execution with dynamic arguments is safe
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)

	// Clean GOCOVERDIR to prevent helper subprocess from corrupting/writing to coverage profile
	env := os.Environ()
	var cleanEnv []string
	for _, e := range env {
		if strings.HasPrefix(e, "GOCOVERDIR=") {
			continue
		}
		cleanEnv = append(cleanEnv, e)
	}
	cleanEnv = append(cleanEnv, "GO_WANT_HELPER_PROCESS=1")
	cmd.Env = cleanEnv
	return cmd
}
