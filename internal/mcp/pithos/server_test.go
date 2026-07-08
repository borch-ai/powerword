package pithos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// helperProcess is used to mock exec.CommandContext
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "no command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "pithos":
		fmt.Printf("pithos %s\n", strings.Join(args, " "))
		if os.Getenv("MOCK_PITHOS_FAIL") == "1" {
			os.Exit(1)
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func mockExecCommand(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	//nolint:gosec // G204: intentional execution of test helper process
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	if fail := os.Getenv("MOCK_PITHOS_FAIL"); fail != "" {
		cmd.Env = append(cmd.Env, "MOCK_PITHOS_FAIL="+fail)
	}
	return cmd
}

func setupTestClientServer(t *testing.T) (*mcp.ClientSession, func()) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect client: %v", err)
	}

	return session, func() {
		_ = session.Close()
		cancel()
	}
}

type initiateTest struct {
	name        string
	args        map[string]interface{}
	mockFail    bool
	wantIsError bool
	wantSubstr  string
}

func TestHandleInitiate(t *testing.T) {
	SetExecCommand(mockExecCommand)
	defer SetExecCommand(exec.CommandContext)

	t.Setenv("POWERWORD_WORKSPACE_ROOT", "/")

	session, cleanup := setupTestClientServer(t)
	defer cleanup()

	tests := []initiateTest{
		{
			name: "success with theme",
			args: map[string]interface{}{
				"project_path": "/tmp/test",
				"theme":        "scifi",
			},
			wantIsError: false,
			wantSubstr:  "pithos initiate --dir /tmp/test --theme scifi",
		},
		{
			name: "success without theme",
			args: map[string]interface{}{
				"project_path": "/tmp/test2",
			},
			wantIsError: false,
			wantSubstr:  "pithos initiate --dir /tmp/test2",
		},
		{
			name: "missing project_path",
			args: map[string]interface{}{
				"theme": "scifi",
			},
			wantIsError: true,
			wantSubstr:  "project_path is required",
		},
		{
			name: "command failure",
			args: map[string]interface{}{
				"project_path": "/tmp/fail",
			},
			mockFail:    true,
			wantIsError: true,
			wantSubstr:  "pithos command failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runInitiateTest(t, tt, session)
		})
	}
}

func runInitiateTest(t *testing.T, tt initiateTest, session *mcp.ClientSession) {
	if tt.mockFail {
		t.Setenv("MOCK_PITHOS_FAIL", "1")
	}

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "pithos_initiate",
		Arguments: tt.args,
	})

	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError != tt.wantIsError {
		t.Errorf("expected IsError %v, got %v", tt.wantIsError, res.IsError)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected content, got none")
	}

	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, tt.wantSubstr) {
		t.Errorf("expected output to contain %q, got %q", tt.wantSubstr, text)
	}
}

type stageTest struct {
	stage       string
	args        map[string]interface{}
	mockFail    bool
	wantIsError bool
	wantSubstr  string
}

func TestHandleStage(t *testing.T) {
	SetExecCommand(mockExecCommand)
	defer SetExecCommand(exec.CommandContext)

	t.Setenv("POWERWORD_WORKSPACE_ROOT", "/")

	session, cleanup := setupTestClientServer(t)
	defer cleanup()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	tests := []stageTest{
		{
			stage: "brew",
			args: map[string]interface{}{
				"project_path": dir1,
			},
			wantIsError: false,
			wantSubstr:  "pithos brew --dir " + dir1,
		},
		{
			stage: "assemble",
			args: map[string]interface{}{
				"project_path": dir2,
			},
			wantIsError: false,
			wantSubstr:  "pithos assemble --dir " + dir2,
		},
		{
			stage:       "missing_path",
			args:        map[string]interface{}{},
			wantIsError: true,
			wantSubstr:  "project_path is required",
		},
		{
			stage: "deploy",
			args: map[string]interface{}{
				"project_path": dir3,
			},
			mockFail:    true,
			wantIsError: true,
			wantSubstr:  "pithos command failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			runStageTest(t, tt, session)
		})
	}
}

func runStageTest(t *testing.T, tt stageTest, session *mcp.ClientSession) {
	if tt.mockFail {
		t.Setenv("MOCK_PITHOS_FAIL", "1")
	}

	var toolName string
	switch tt.stage {
	case "assemble":
		toolName = "pithos_assemble"
	case "deploy":
		toolName = "pithos_deploy"
	default:
		toolName = "pithos_brew"
	}

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: tt.args,
	})

	if err != nil {
		if tt.wantIsError && strings.Contains(err.Error(), "required") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IsError != tt.wantIsError {
		t.Errorf("expected IsError %v, got %v", tt.wantIsError, res.IsError)
	}

	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, tt.wantSubstr) {
		t.Errorf("expected output to contain %q, got %q", tt.wantSubstr, text)
	}
}

func TestSetupServer(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestSetExecCommand(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()
	SetExecCommand(exec.CommandContext)
}

func TestCheckSandbox(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tmpDir)

	if err := checkSandbox(filepath.Join(tmpDir, "ok")); err != nil {
		t.Errorf("expected no error for sub-path, got %v", err)
	}

	if err := checkSandbox("/some/path/outside"); err == nil {
		t.Errorf("expected error for outside path, got none")
	}

	t.Setenv("POWERWORD_WORKSPACE_ROOT", "")
	cwd, _ := os.Getwd()
	if err := checkSandbox(filepath.Join(cwd, "ok")); err != nil {
		t.Errorf("expected no error for cwd fallback, got %v", err)
	}

	// Create a real symlink that points outside
	outsideDir := t.TempDir()
	insideSymlink := filepath.Join(tmpDir, "symlink")
	if err := os.Symlink(outsideDir, insideSymlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tmpDir)

	if err := checkSandbox(insideSymlink); err == nil {
		t.Errorf("expected error for symlink pointing outside, got none")
	}
}
