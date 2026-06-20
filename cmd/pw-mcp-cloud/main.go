package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/cloud"
	"github.com/borch-ai/powerword/pkg/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workspaceRoot := os.Getenv("POWERWORD_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get cwd: %w", err)
		}
		workspaceRoot = cwd
	}

	cfg, err := config.LoadFromWorkspace(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	srv, err := setupServer(workspaceRoot, cfg, nil)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const (
	listInstancesSchema = `{
		"type": "object",
		"properties": {
			"provider": {
				"type": "string",
				"description": "The cloud provider to query ('aws' or 'gcp')."
			},
			"zone": {
				"type": "string",
				"description": "Optional zone to filter by (GCP only, e.g. 'us-central1-a')."
			},
			"region": {
				"type": "string",
				"description": "Optional region to filter by (AWS only, e.g. 'us-west-2')."
			},
			"tags": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				},
				"description": "Optional key-value tag filters."
			}
		},
		"required": ["provider"]
	}`

	getLogsSchema = `{
		"type": "object",
		"properties": {
			"provider": {
				"type": "string",
				"description": "The cloud provider log source ('aws' or 'gcp')."
			},
			"region": {
				"type": "string",
				"description": "Optional region to query (AWS only, e.g. 'us-east-1')."
			},
			"log_group": {
				"type": "string",
				"description": "The log group name (AWS CloudWatch logGroupName) or log name (GCP logName, e.g. 'stdout')."
			},
			"log_stream": {
				"type": "string",
				"description": "Optional log stream name (AWS CloudWatch only)."
			},
			"limit": {
				"type": "integer",
				"description": "Optional maximum number of logs to retrieve (default: 50)."
			}
		},
		"required": ["provider", "log_group"]
	}`

	checkBucketSchema = `{
		"type": "object",
		"properties": {
			"provider": {
				"type": "string",
				"description": "Optional cloud provider to check ('aws' or 'gcp'). Defaults to configuration."
			},
			"region": {
				"type": "string",
				"description": "Optional region (AWS only, e.g. 'us-east-1')."
			},
			"bucket_name": {
				"type": "string",
				"description": "Optional bucket name to check. Defaults to configuration."
			}
		}
	}`

	uploadFileSchema = `{
		"type": "object",
		"properties": {
			"local_path": {
				"type": "string",
				"description": "The absolute path of the local file to upload."
			}
		},
		"required": ["local_path"]
	}`

	listRunServicesSchema = `{
		"type": "object",
		"properties": {
			"region": {
				"type": "string",
				"description": "The GCP region to query (e.g. 'us-central1')."
			}
		},
		"required": ["region"]
	}`

	getRunServiceSchema = `{
		"type": "object",
		"properties": {
			"region": {
				"type": "string",
				"description": "The GCP region where the service resides (e.g. 'us-central1')."
			},
			"service_name": {
				"type": "string",
				"description": "The name of the Cloud Run service."
			}
		},
		"required": ["region", "service_name"]
	}`

	deployRunServiceSchema = `{
		"type": "object",
		"properties": {
			"region": {
				"type": "string",
				"description": "The GCP region to deploy to (e.g. 'us-central1')."
			},
			"service_name": {
				"type": "string",
				"description": "The name of the Cloud Run service to deploy."
			},
			"image": {
				"type": "string",
				"description": "The container image URL to deploy."
			},
			"env_vars": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				},
				"description": "Optional environment variables map."
			},
			"concurrency": {
				"type": "integer",
				"description": "Optional container concurrency limit."
			},
			"cpu": {
				"type": "string",
				"description": "Optional CPU limit (e.g. '1', '2')."
			},
			"memory": {
				"type": "string",
				"description": "Optional memory limit (e.g. '256Mi', '512Mi', '1Gi')."
			}
		},
		"required": ["region", "service_name", "image"]
	}`
)

func setupServer(workspaceRoot string, cfg *config.Config, svc *cloud.CloudService) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-cloud",
		Version: "0.1.0",
	}, nil)

	cloudService := svc
	if cloudService == nil {
		cloudService = cloud.NewCloudService(cfg, nil, nil, nil, nil, nil, nil)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_list_instances",
		Description: "Retrieves the state of cloud virtual machine instances (EC2/GCE) filtered by tags or status.",
		InputSchema: json.RawMessage(listInstancesSchema),
	}, handleListInstances(cloudService))

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_get_logs",
		Description: "Retrieves container or infrastructure log streams (CloudWatch/Stackdriver).",
		InputSchema: json.RawMessage(getLogsSchema),
	}, handleGetLogs(cloudService))

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_check_bucket",
		Description: "Verifies public bucket configuration and checks basic object metadata.",
		InputSchema: json.RawMessage(checkBucketSchema),
	}, handleCheckBucket(cloudService))

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_upload_file",
		Description: "Uploads a local file to the configured cloud storage bucket.",
		InputSchema: json.RawMessage(uploadFileSchema),
	}, handleUploadFile(cloudService))

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_list_run_services",
		Description: "Retrieves the state of Cloud Run services in the target region.",
		InputSchema: json.RawMessage(listRunServicesSchema),
	}, handleListRunServices(cloudService))

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_get_run_service",
		Description: "Retrieves metadata for a specific Cloud Run service in the target region.",
		InputSchema: json.RawMessage(getRunServiceSchema),
	}, handleGetRunService(cloudService))

	srv.AddTool(&mcp.Tool{
		Name:        "cloud_deploy_run_service",
		Description: "Deploys (creates or updates) a serverless container on GCP Cloud Run.",
		InputSchema: json.RawMessage(deployRunServiceSchema),
	}, handleDeployRunService(cloudService))

	return srv, nil
}

func handleListInstances(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Provider string            `json:"provider"`
			Zone     string            `json:"zone"`
			Region   string            `json:"region"`
			Tags     map[string]string `json:"tags"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Provider == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "provider parameter is required"}},
			}, nil
		}

		res, err := svc.ListInstances(ctx, args.Provider, args.Zone, args.Region, args.Tags)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list instances: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleGetLogs(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Provider  string `json:"provider"`
			Region    string `json:"region"`
			LogGroup  string `json:"log_group"`
			LogStream string `json:"log_stream"`
			Limit     int    `json:"limit"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Provider == "" || args.LogGroup == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "provider and log_group parameters are required"}},
			}, nil
		}

		res, err := svc.GetLogs(ctx, args.Provider, args.Region, args.LogGroup, args.LogStream, args.Limit)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to retrieve logs: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleCheckBucket(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Provider   string `json:"provider"`
			Region     string `json:"region"`
			BucketName string `json:"bucket_name"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		res, err := svc.CheckBucket(ctx, args.Provider, args.Region, args.BucketName)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to check bucket: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleUploadFile(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			LocalPath string `json:"local_path"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.LocalPath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "local_path parameter is required"}},
			}, nil
		}

		if !filepath.IsAbs(args.LocalPath) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "local_path must be an absolute path"}},
			}, nil
		}

		url, err := svc.UploadFile(ctx, args.LocalPath)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to upload file: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: url}},
		}, nil
	}
}

func handleListRunServices(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Region string `json:"region"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Region == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "region parameter is required"}},
			}, nil
		}

		res, err := svc.ListRunServices(ctx, args.Region)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list Cloud Run services: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleGetRunService(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Region      string `json:"region"`
			ServiceName string `json:"service_name"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Region == "" || args.ServiceName == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "region and service_name parameters are required"}},
			}, nil
		}

		res, err := svc.GetRunService(ctx, args.Region, args.ServiceName)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get Cloud Run service: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleDeployRunService(svc *cloud.CloudService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Region      string            `json:"region"`
			ServiceName string            `json:"service_name"`
			Image       string            `json:"image"`
			EnvVars     map[string]string `json:"env_vars"`
			Concurrency int64             `json:"concurrency"`
			CPU         string            `json:"cpu"`
			Memory      string            `json:"memory"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Region == "" || args.ServiceName == "" || args.Image == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "region, service_name, and image parameters are required"}},
			}, nil
		}

		res, err := svc.DeployRunService(ctx, args.Region, args.ServiceName, args.Image, args.EnvVars, args.Concurrency, args.CPU, args.Memory)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to deploy Cloud Run service: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}
