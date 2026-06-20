package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/cloud"
	"github.com/borch-ai/powerword/pkg/config"
)

// Mock client interfaces for main_test

type mockEC2Client struct {
	DescribeInstancesFunc func(ctx context.Context, region string, tags map[string]string) ([]cloud.Instance, error)
}

func (m *mockEC2Client) DescribeInstances(ctx context.Context, region string, tags map[string]string) ([]cloud.Instance, error) {
	if m.DescribeInstancesFunc != nil {
		return m.DescribeInstancesFunc(ctx, region, tags)
	}
	return nil, nil
}

type mockGCEClient struct {
	ListInstancesFunc func(ctx context.Context, projectID, zone string, tags map[string]string) ([]cloud.Instance, error)
}

func (m *mockGCEClient) ListInstances(ctx context.Context, projectID, zone string, tags map[string]string) ([]cloud.Instance, error) {
	if m.ListInstancesFunc != nil {
		return m.ListInstancesFunc(ctx, projectID, zone, tags)
	}
	return nil, nil
}

type mockCWLogsClient struct {
	GetLogEventsFunc func(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]cloud.LogEvent, error)
}

func (m *mockCWLogsClient) GetLogEvents(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]cloud.LogEvent, error) {
	if m.GetLogEventsFunc != nil {
		return m.GetLogEventsFunc(ctx, region, logGroupName, logStreamName, limit)
	}
	return nil, nil
}

type mockS3Client struct {
	CheckBucketFunc func(ctx context.Context, region, bucketName string) (*cloud.BucketMetadata, error)
}

func (m *mockS3Client) CheckBucket(ctx context.Context, region, bucketName string) (*cloud.BucketMetadata, error) {
	if m.CheckBucketFunc != nil {
		return m.CheckBucketFunc(ctx, region, bucketName)
	}
	return nil, nil
}

type mockGCPRunClient struct {
	ListServicesFunc  func(ctx context.Context, projectID, region string) ([]cloud.CloudRunService, error)
	GetServiceFunc    func(ctx context.Context, projectID, region, serviceName string) (*cloud.CloudRunService, error)
	DeployServiceFunc func(ctx context.Context, projectID, region, serviceName, image string, envVars map[string]string, concurrency int64, cpu, memory string) (*cloud.CloudRunService, error)
}

func (m *mockGCPRunClient) ListServices(ctx context.Context, projectID, region string) ([]cloud.CloudRunService, error) {
	if m.ListServicesFunc != nil {
		return m.ListServicesFunc(ctx, projectID, region)
	}
	return nil, nil
}

func (m *mockGCPRunClient) GetService(ctx context.Context, projectID, region, serviceName string) (*cloud.CloudRunService, error) {
	if m.GetServiceFunc != nil {
		return m.GetServiceFunc(ctx, projectID, region, serviceName)
	}
	return nil, nil
}

func (m *mockGCPRunClient) DeployService(ctx context.Context, projectID, region, serviceName, image string, envVars map[string]string, concurrency int64, cpu, memory string) (*cloud.CloudRunService, error) {
	if m.DeployServiceFunc != nil {
		return m.DeployServiceFunc(ctx, projectID, region, serviceName, image, envVars, concurrency, cpu, memory)
	}
	return nil, nil
}

func assertResponse(t *testing.T, res *mcp.CallToolResult, wantError bool, wantSubstr string) {
	t.Helper()
	if res.IsError != wantError {
		t.Errorf("expected error: %v, got: %v (content: %v)", wantError, res.IsError, res.Content)
	}
	if len(res.Content) == 0 {
		t.Errorf("no content in response")
		return
	}
	var contentStr string
	if txt, ok := res.Content[0].(*mcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(res.Content[0])
	}
	if !strings.Contains(contentStr, wantSubstr) {
		t.Errorf("expected response to contain %q, got %q", wantSubstr, contentStr)
	}
}

func startTestServer(t *testing.T, workspaceRoot string, svc *cloud.CloudService) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	cfg := &config.Config{}
	srv, err := setupServer(workspaceRoot, cfg, svc)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect client: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
		cancel()
	}
	return session, ctx, cleanup
}

func TestCloud_MCP_ListInstances(t *testing.T) {
	ec2Mock := &mockEC2Client{
		DescribeInstancesFunc: func(ctx context.Context, region string, tags map[string]string) ([]cloud.Instance, error) {
			return []cloud.Instance{
				{ID: "i-12345", Name: "my-aws-instance", Provider: "aws", State: "running"},
			}, nil
		},
	}
	gceMock := &mockGCEClient{}

	cfg := &config.Config{}
	svc := cloud.NewCloudService(cfg, ec2Mock, gceMock, nil, nil, nil, nil)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: AWS DescribeInstances Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_list_instances",
		Arguments: json.RawMessage(`{
			"provider": "aws",
			"region": "us-west-2"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_list_instances failed: %v", err)
	}
	assertResponse(t, res, false, "my-aws-instance")

	// Test 2: Error Call (missing provider)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_list_instances",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_list_instances error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "provider parameter is required")
}

func TestCloud_MCP_GetLogs(t *testing.T) {
	cwMock := &mockCWLogsClient{
		GetLogEventsFunc: func(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]cloud.LogEvent, error) {
			return []cloud.LogEvent{
				{Timestamp: 12345, Message: "AWS Log Entry"},
			}, nil
		},
	}

	cfg := &config.Config{}
	svc := cloud.NewCloudService(cfg, nil, nil, cwMock, nil, nil, nil)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: GetLogs Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_get_logs",
		Arguments: json.RawMessage(`{
			"provider": "aws",
			"log_group": "my-log-group"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_get_logs failed: %v", err)
	}
	assertResponse(t, res, false, "AWS Log Entry")

	// Test 2: Error Call (missing required fields)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_get_logs",
		Arguments: json.RawMessage(`{"provider": "aws"}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_get_logs error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "provider and log_group parameters are required")
}

func TestCloud_MCP_CheckBucket(t *testing.T) {
	s3Mock := &mockS3Client{
		CheckBucketFunc: func(ctx context.Context, region, bucketName string) (*cloud.BucketMetadata, error) {
			return &cloud.BucketMetadata{
				Name:     bucketName,
				Provider: "aws",
				Exists:   true,
				Location: "us-east-1",
			}, nil
		},
	}

	cfg := &config.Config{}
	svc := cloud.NewCloudService(cfg, nil, nil, nil, nil, s3Mock, nil)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: CheckBucket Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_check_bucket",
		Arguments: json.RawMessage(`{
			"provider": "aws",
			"bucket_name": "my-s3-bucket"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_check_bucket failed: %v", err)
	}
	assertResponse(t, res, false, "my-s3-bucket")
}

type mockMainUploader struct {
	UploadFileFunc func(ctx context.Context, localPath string) (string, error)
}

func (m *mockMainUploader) UploadFile(ctx context.Context, localPath string) (string, error) {
	if m.UploadFileFunc != nil {
		return m.UploadFileFunc(ctx, localPath)
	}
	return "", nil
}

func TestCloud_MCP_UploadFile(t *testing.T) {
	cfg := &config.Config{}
	svc := cloud.NewCloudService(cfg, nil, nil, nil, nil, nil, nil)
	mu := &mockMainUploader{
		UploadFileFunc: func(ctx context.Context, localPath string) (string, error) {
			if localPath != "/absolute/path/file.txt" {
				return "", fmt.Errorf("unexpected localPath: %s", localPath)
			}
			return "https://my-bucket-url.com/file.txt", nil
		},
	}
	svc.SetUploader(mu)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: UploadFile Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_upload_file",
		Arguments: json.RawMessage(`{
			"local_path": "/absolute/path/file.txt"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_upload_file failed: %v", err)
	}
	assertResponse(t, res, false, "https://my-bucket-url.com/file.txt")

	// Test 2: Error Call (missing local_path)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_upload_file",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_upload_file error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "local_path parameter is required")

	// Test 3: Error Call (relative local_path)
	relRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_upload_file",
		Arguments: json.RawMessage(`{
			"local_path": "relative/path/file.txt"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool cloud_upload_file error call failed: %v", err)
	}
	assertResponse(t, relRes, true, "local_path must be an absolute path")
}

func TestCloud_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, nil)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_list_instances",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_get_logs",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_check_bucket",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_upload_file",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_list_run_services",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_get_run_service",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_deploy_run_service",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestCloud_MCP_CloudRunServices(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

	runMock := &mockGCPRunClient{
		ListServicesFunc: func(ctx context.Context, projectID, region string) ([]cloud.CloudRunService, error) {
			return []cloud.CloudRunService{{Name: "run-service", URL: "https://url"}}, nil
		},
		GetServiceFunc: func(ctx context.Context, projectID, region, serviceName string) (*cloud.CloudRunService, error) {
			return &cloud.CloudRunService{Name: serviceName, URL: "https://url"}, nil
		},
		DeployServiceFunc: func(ctx context.Context, projectID, region, serviceName, image string, envVars map[string]string, concurrency int64, cpu, memory string) (*cloud.CloudRunService, error) {
			return &cloud.CloudRunService{Name: serviceName, URL: "https://url", Image: image, Concurrency: concurrency}, nil
		},
	}

	cfg := &config.Config{}
	svc := cloud.NewCloudService(cfg, nil, nil, nil, nil, nil, nil)
	svc.SetGCPRunClient(runMock)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// 1. List
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_list_run_services",
		Arguments: json.RawMessage(`{
			"region": "us-central1"
		}`),
	})
	if err != nil {
		t.Fatalf("cloud_list_run_services failed: %v", err)
	}
	assertResponse(t, res, false, "run-service")

	// Missing region
	resErr, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "cloud_list_run_services",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("cloud_list_run_services failed: %v", err)
	}
	assertResponse(t, resErr, true, "region parameter is required")

	// 2. Get
	resGet, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_get_run_service",
		Arguments: json.RawMessage(`{
			"region": "us-central1",
			"service_name": "my-service"
		}`),
	})
	if err != nil {
		t.Fatalf("cloud_get_run_service failed: %v", err)
	}
	assertResponse(t, resGet, false, "my-service")

	// Missing service name
	resGetErr, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_get_run_service",
		Arguments: json.RawMessage(`{
			"region": "us-central1"
		}`),
	})
	if err != nil {
		t.Fatalf("cloud_get_run_service failed: %v", err)
	}
	assertResponse(t, resGetErr, true, "region and service_name parameters are required")

	// 3. Deploy
	resDeploy, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_deploy_run_service",
		Arguments: json.RawMessage(`{
			"region": "us-central1",
			"service_name": "my-service",
			"image": "gcr.io/image:latest",
			"concurrency": 80
		}`),
	})
	if err != nil {
		t.Fatalf("cloud_deploy_run_service failed: %v", err)
	}
	assertResponse(t, resDeploy, false, "gcr.io/image:latest")

	// Missing required image
	resDeployErr, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "cloud_deploy_run_service",
		Arguments: json.RawMessage(`{
			"region": "us-central1",
			"service_name": "my-service"
		}`),
	})
	if err != nil {
		t.Fatalf("cloud_deploy_run_service failed: %v", err)
	}
	assertResponse(t, resDeployErr, true, "region, service_name, and image parameters are required")
}

func TestRun_ConfigParsing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	srv, err := setupServer(tempDir, nil, nil)
	if err != nil {
		t.Fatalf("setupServer failed: %v", err)
	}
	if srv == nil {
		t.Error("expected setupServer to return a server")
	}

	// Write bad config to force run() error
	badConfigPath := tempDir + "/powerword.toml"
	_ = os.WriteFile(badConfigPath, []byte("bad config format"), 0600)
	err = run()
	if err == nil {
		t.Error("expected run to fail with invalid config file, got nil")
	}
}

func TestRun_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	// Write mock config file
	cfgTOML := `
[plugins.cloud]
provider = "noop"
`
	_ = os.WriteFile(tempDir+"/powerword.toml", []byte(cfgTOML), 0600)

	// Mock stdin/stdout to avoid blocking
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	_ = w.Close() // Close write end so read returns EOF immediately

	// Pipe stdout to null
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = null
	defer func() {
		_ = null.Close()
	}()

	err = run()
	// Should exit cleanly (or exit with EOF related error which is expected and handled)
	if err != nil && !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "broken pipe") {
		t.Logf("run() exited with: %v", err)
	}
}
