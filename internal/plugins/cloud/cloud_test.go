package cloud

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

// Mock Client Implementations

type mockEC2Client struct {
	DescribeInstancesFunc func(ctx context.Context, region string, tags map[string]string) ([]Instance, error)
}

func (m *mockEC2Client) DescribeInstances(ctx context.Context, region string, tags map[string]string) ([]Instance, error) {
	if m.DescribeInstancesFunc != nil {
		return m.DescribeInstancesFunc(ctx, region, tags)
	}
	return nil, nil
}

type mockGCEClient struct {
	ListInstancesFunc func(ctx context.Context, projectID, zone string, tags map[string]string) ([]Instance, error)
}

func (m *mockGCEClient) ListInstances(ctx context.Context, projectID, zone string, tags map[string]string) ([]Instance, error) {
	if m.ListInstancesFunc != nil {
		return m.ListInstancesFunc(ctx, projectID, zone, tags)
	}
	return nil, nil
}

type mockCWLogsClient struct {
	GetLogEventsFunc func(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]LogEvent, error)
}

func (m *mockCWLogsClient) GetLogEvents(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]LogEvent, error) {
	if m.GetLogEventsFunc != nil {
		return m.GetLogEventsFunc(ctx, region, logGroupName, logStreamName, limit)
	}
	return nil, nil
}

type mockGCPLogClient struct {
	GetLogEntriesFunc func(ctx context.Context, projectID, logName string, limit int) ([]LogEvent, error)
}

func (m *mockGCPLogClient) GetLogEntries(ctx context.Context, projectID, logName string, limit int) ([]LogEvent, error) {
	if m.GetLogEntriesFunc != nil {
		return m.GetLogEntriesFunc(ctx, projectID, logName, limit)
	}
	return nil, nil
}

type mockS3Client struct {
	CheckBucketFunc func(ctx context.Context, region, bucketName string) (*BucketMetadata, error)
}

func (m *mockS3Client) CheckBucket(ctx context.Context, region, bucketName string) (*BucketMetadata, error) {
	if m.CheckBucketFunc != nil {
		return m.CheckBucketFunc(ctx, region, bucketName)
	}
	return nil, nil
}

type mockGCSClient struct {
	CheckBucketFunc func(ctx context.Context, projectID, bucketName string) (*BucketMetadata, error)
}

func (m *mockGCSClient) CheckBucket(ctx context.Context, projectID, bucketName string) (*BucketMetadata, error) {
	if m.CheckBucketFunc != nil {
		return m.CheckBucketFunc(ctx, projectID, bucketName)
	}
	return nil, nil
}

func TestListInstances(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Cloud: config.CloudConfig{
				Region: "us-west-2",
			},
		},
	}

	tests := []struct {
		name     string
		provider string
		zone     string
		region   string
		tags     map[string]string
		setup    func(*mockEC2Client, *mockGCEClient)
		want     []Instance
		wantErr  bool
	}{
		{
			name:     "AWS DescribeInstances Success",
			provider: "aws",
			setup: func(ec2 *mockEC2Client, gce *mockGCEClient) {
				ec2.DescribeInstancesFunc = func(ctx context.Context, region string, tags map[string]string) ([]Instance, error) {
					if region != "us-west-2" {
						return nil, errors.New("wrong region")
					}
					return []Instance{
						{ID: "i-12345", Name: "test-instance", Provider: "aws", State: "running"},
					}, nil
				}
			},
			want: []Instance{
				{ID: "i-12345", Name: "test-instance", Provider: "aws", State: "running"},
			},
			wantErr: false,
		},
		{
			name:     "GCP ListInstances Success",
			provider: "gcp",
			zone:     "us-central1-a",
			setup: func(ec2 *mockEC2Client, gce *mockGCEClient) {
				gce.ListInstancesFunc = func(ctx context.Context, projectID, zone string, tags map[string]string) ([]Instance, error) {
					if zone != "us-central1-a" {
						return nil, errors.New("wrong zone")
					}
					return []Instance{
						{ID: "54321", Name: "gcp-instance", Provider: "gcp", State: "RUNNING"},
					}, nil
				}
			},
			want: []Instance{
				{ID: "54321", Name: "gcp-instance", Provider: "gcp", State: "RUNNING"},
			},
			wantErr: false,
		},
		{
			name:     "Unsupported provider error",
			provider: "azure",
			setup:    func(ec2 *mockEC2Client, gce *mockGCEClient) {},
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable to pass validation for GCP project
			t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

			ec2Mock := &mockEC2Client{}
			gceMock := &mockGCEClient{}
			tt.setup(ec2Mock, gceMock)

			svc := NewCloudService(cfg, ec2Mock, gceMock, nil, nil, nil, nil)
			got, err := svc.ListInstances(context.Background(), tt.provider, tt.zone, tt.region, tt.tags)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ListInstances() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ListInstances() = %v, want = %v", got, tt.want)
			}
		})
	}
}

func TestGetLogs(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Cloud: config.CloudConfig{
				Region: "us-east-1",
			},
		},
	}

	tests := []struct {
		name      string
		provider  string
		logGroup  string
		logStream string
		limit     int
		setup     func(*mockCWLogsClient, *mockGCPLogClient)
		want      []LogEvent
		wantErr   bool
	}{
		{
			name:      "AWS CloudWatch logs success",
			provider:  "aws",
			logGroup:  "/aws/lambda/test",
			logStream: "stream-1",
			limit:     10,
			setup: func(cw *mockCWLogsClient, gcp *mockGCPLogClient) {
				cw.GetLogEventsFunc = func(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]LogEvent, error) {
					return []LogEvent{
						{Timestamp: 1623880000, Message: "AWS Log Entry"},
					}, nil
				}
			},
			want: []LogEvent{
				{Timestamp: 1623880000, Message: "AWS Log Entry"},
			},
			wantErr: false,
		},
		{
			name:     "GCP Logging success",
			provider: "gcp",
			logGroup: "syslog",
			limit:    10,
			setup: func(cw *mockCWLogsClient, gcp *mockGCPLogClient) {
				gcp.GetLogEntriesFunc = func(ctx context.Context, projectID, logName string, limit int) ([]LogEvent, error) {
					return []LogEvent{
						{Timestamp: 0, Message: "GCP Log Entry"},
					}, nil
				}
			},
			want: []LogEvent{
				{Timestamp: 0, Message: "GCP Log Entry"},
			},
			wantErr: false,
		},
		{
			name:     "Missing AWS logGroup error",
			provider: "aws",
			setup:    func(cw *mockCWLogsClient, gcp *mockGCPLogClient) {},
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

			cwMock := &mockCWLogsClient{}
			gcpMock := &mockGCPLogClient{}
			tt.setup(cwMock, gcpMock)

			svc := NewCloudService(cfg, nil, nil, cwMock, gcpMock, nil, nil)
			got, err := svc.GetLogs(context.Background(), tt.provider, "us-east-1", tt.logGroup, tt.logStream, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetLogs() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetLogs() = %v, want = %v", got, tt.want)
			}
		})
	}
}

func TestCheckBucket(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Cloud: config.CloudConfig{
				Provider: "noop",
				Bucket:   "default-bucket",
			},
		},
	}

	tests := []struct {
		name       string
		provider   string
		bucketName string
		setup      func(*mockS3Client, *mockGCSClient)
		want       *BucketMetadata
		wantErr    bool
	}{
		{
			name:       "NoOp provider returns clean response",
			provider:   "noop",
			bucketName: "my-bucket",
			setup:      func(s3 *mockS3Client, gcs *mockGCSClient) {},
			want: &BucketMetadata{
				Name:     "my-bucket",
				Provider: "noop",
				Exists:   false,
			},
			wantErr: false,
		},
		{
			name:       "AWS S3 bucket check success",
			provider:   "aws",
			bucketName: "s3-bucket",
			setup: func(s3 *mockS3Client, gcs *mockGCSClient) {
				s3.CheckBucketFunc = func(ctx context.Context, region, bucketName string) (*BucketMetadata, error) {
					return &BucketMetadata{
						Name:     bucketName,
						Provider: "aws",
						Exists:   true,
						Location: "us-west-2",
					}, nil
				}
			},
			want: &BucketMetadata{
				Name:     "s3-bucket",
				Provider: "aws",
				Exists:   true,
				Location: "us-west-2",
			},
			wantErr: false,
		},
		{
			name:       "GCP GCS bucket check success",
			provider:   "gcp",
			bucketName: "gcs-bucket",
			setup: func(s3 *mockS3Client, gcs *mockGCSClient) {
				gcs.CheckBucketFunc = func(ctx context.Context, projectID, bucketName string) (*BucketMetadata, error) {
					return &BucketMetadata{
						Name:     bucketName,
						Provider: "gcp",
						Exists:   true,
						Location: "US",
					}, nil
				}
			},
			want: &BucketMetadata{
				Name:     "gcs-bucket",
				Provider: "gcp",
				Exists:   true,
				Location: "US",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

			s3Mock := &mockS3Client{}
			gcsMock := &mockGCSClient{}
			tt.setup(s3Mock, gcsMock)

			svc := NewCloudService(cfg, nil, nil, nil, nil, s3Mock, gcsMock)
			got, err := svc.CheckBucket(context.Background(), tt.provider, "us-east-1", tt.bucketName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckBucket() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckBucket() = %v, want = %v", got, tt.want)
			}
		})
	}
}

//nolint:gocognit,funlen
func TestRealClientsWithMockHTTP(t *testing.T) {
	// 1. Start a mock server to capture GCP and AWS SDK calls
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("--- MOCK REQUEST: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")

		// GCP Compute GCE List
		if strings.Contains(r.URL.Path, "/projects/") && strings.Contains(r.URL.Path, "/instances") {
			resp := `{
				"kind": "compute#instanceList",
				"items": [
					{
						"id": "12345",
						"name": "gcp-instance",
						"status": "RUNNING",
						"machineType": "machineTypes/n1-standard-1",
						"zone": "zones/us-central1-a",
						"networkInterfaces": [{"networkIP": "10.0.0.2"}]
					}
				]
			}`
			_, _ = w.Write([]byte(resp))
			return
		}

		// GCP Storage Bucket Check
		if strings.Contains(r.URL.Path, "/b/") {
			resp := `{
				"name": "gcs-bucket",
				"location": "US",
				"labels": {"env": "prod"}
			}`
			_, _ = w.Write([]byte(resp))
			return
		}

		// GCP Logging entries list
		if strings.Contains(r.URL.Path, "/v2/entries:list") {
			resp := `{
				"entries": [
					{"textPayload": "GCP Log Entry"}
				]
			}`
			_, _ = w.Write([]byte(resp))
			return
		}

		// AWS POST requests (EC2 / CW Logs)
		if r.Method == "POST" && r.URL.Path == "/" {
			body, _ := io.ReadAll(r.Body)
			bodyStr := string(body)
			if strings.Contains(bodyStr, "Action=DescribeInstances") {
				w.Header().Set("Content-Type", "text/xml")
				resp := `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
					<reservationSet>
						<item>
							<instancesSet>
								<item>
									<instanceId>i-12345</instanceId>
									<instanceType>t2.micro</instanceType>
									<instanceState>
										<code>16</code>
										<name>running</name>
									</instanceState>
									<privateIpAddress>10.0.0.1</privateIpAddress>
									<tagSet>
										<item>
											<key>Name</key>
											<value>test-instance</value>
										</item>
									</tagSet>
								</item>
							</instancesSet>
						</item>
					</reservationSet>
				</DescribeInstancesResponse>`
				_, _ = w.Write([]byte(resp))
				return
			}

			target := r.Header.Get("X-Amz-Target")
			if strings.Contains(target, "GetLogEvents") {
				resp := `{
					"events": [
						{"timestamp": 1234567, "message": "AWS Log Entry"}
					]
				}`
				_, _ = w.Write([]byte(resp))
				return
			}
		}

		// AWS S3 Requests
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.RawQuery, "location") {
			w.Header().Set("Content-Type", "text/xml")
			resp := `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-west-2</LocationConstraint>`
			_, _ = w.Write([]byte(resp))
			return
		}
		if r.Method == "GET" && strings.Contains(r.URL.RawQuery, "tagging") {
			w.Header().Set("Content-Type", "text/xml")
			resp := `<Tagging xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
				<TagSet>
					<Tag>
						<Key>env</Key>
						<Value>prod</Value>
					</Tag>
				</TagSet>
			</Tagging>`
			_, _ = w.Write([]byte(resp))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Set redirect env vars
	t.Setenv("POWERWORD_CLOUD_MOCK_ENDPOINT", server.URL)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

	cfg := &config.Config{}
	cfg.Plugins.Cloud.Region = "us-east-1"
	cfg.Plugins.Cloud.Bucket = "test-bucket"

	ctx := context.Background()

	// 2. Test Real Clients
	ec2 := &realEC2Client{cfg: cfg}
	instances, err := ec2.DescribeInstances(ctx, "us-east-1", map[string]string{"env": "prod"})
	if err != nil {
		t.Fatalf("realEC2Client failed: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != "i-12345" {
		t.Errorf("unexpected EC2 instance: %+v", instances)
	}

	gce := &realGCEClient{cfg: cfg}
	gceInsts, err := gce.ListInstances(ctx, "test-project-123", "us-central1-a", map[string]string{"env": "prod"})
	if err != nil {
		t.Fatalf("realGCEClient failed: %v", err)
	}
	if len(gceInsts) != 1 || gceInsts[0].Name != "gcp-instance" {
		t.Errorf("unexpected GCE instance: %+v", gceInsts)
	}

	cw := &realCWLogsClient{cfg: cfg}
	logs, err := cw.GetLogEvents(ctx, "us-east-1", "group", "stream", 10)
	if err != nil {
		t.Fatalf("realCWLogsClient failed: %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "AWS Log Entry" {
		t.Errorf("unexpected CW logs: %+v", logs)
	}

	gcpLog := &realGCPLogClient{cfg: cfg}
	gcpEntries, err := gcpLog.GetLogEntries(ctx, "test-project-123", "stdout", 10)
	if err != nil {
		t.Fatalf("realGCPLogClient failed: %v", err)
	}
	if len(gcpEntries) != 1 || gcpEntries[0].Message != "GCP Log Entry" {
		t.Errorf("unexpected GCP log entries: %+v", gcpEntries)
	}

	s3c := &realS3Client{cfg: cfg}
	s3Meta, err := s3c.CheckBucket(ctx, "us-east-1", "test-bucket")
	if err != nil {
		t.Fatalf("realS3Client failed: %v", err)
	}
	if !s3Meta.Exists || s3Meta.Location != "us-west-2" || s3Meta.Tags["env"] != "prod" {
		t.Errorf("unexpected S3 metadata: %+v", s3Meta)
	}

	gcsc := &realGCSClient{cfg: cfg}
	gcsMeta, err := gcsc.CheckBucket(ctx, "test-project-123", "test-bucket")
	if err != nil {
		t.Fatalf("realGCSClient failed: %v", err)
	}
	if !gcsMeta.Exists || gcsMeta.Location != "US" || gcsMeta.Tags["env"] != "prod" {
		t.Errorf("unexpected GCS metadata: %+v", gcsMeta)
	}

	// 3. Test Service high-level methods with real providers enabled
	svc := NewCloudService(cfg, nil, nil, nil, nil, nil, nil)
	insts, err := svc.ListInstances(ctx, "aws", "zone", "us-east-1", nil)
	if err != nil {
		t.Fatalf("svc.ListInstances (aws) failed: %v", err)
	}
	if len(insts) != 1 {
		t.Errorf("expected 1 instance, got %d", len(insts))
	}

	gcpInsts2, err := svc.ListInstances(ctx, "gcp", "us-central1-a", "", nil)
	if err != nil {
		t.Fatalf("svc.ListInstances (gcp) failed: %v", err)
	}
	if len(gcpInsts2) != 1 {
		t.Errorf("expected 1 instance, got %d", len(gcpInsts2))
	}

	cwLogs2, err := svc.GetLogs(ctx, "aws", "us-east-1", "group", "stream", 10)
	if err != nil {
		t.Fatalf("svc.GetLogs (aws) failed: %v", err)
	}
	if len(cwLogs2) != 1 {
		t.Errorf("expected 1 log, got %d", len(cwLogs2))
	}

	gcpLogs2, err := svc.GetLogs(ctx, "gcp", "", "stdout", "", 10)
	if err != nil {
		t.Fatalf("svc.GetLogs (gcp) failed: %v", err)
	}
	if len(gcpLogs2) != 1 {
		t.Errorf("expected 1 log, got %d", len(gcpLogs2))
	}

	s3Bucket2, err := svc.CheckBucket(ctx, "aws", "us-east-1", "test-bucket")
	if err != nil {
		t.Fatalf("svc.CheckBucket (aws) failed: %v", err)
	}
	if !s3Bucket2.Exists {
		t.Errorf("expected bucket to exist")
	}

	gcsBucket2, err := svc.CheckBucket(ctx, "gcp", "", "test-bucket")
	if err != nil {
		t.Fatalf("svc.CheckBucket (gcp) failed: %v", err)
	}
	if !gcsBucket2.Exists {
		t.Errorf("expected bucket to exist")
	}
}

func TestAWSAndGCPConfigHelpers(t *testing.T) {
	// Create temp directory for dummy credentials
	tempDir := t.TempDir()

	// 1. Test AWS credentials file parsing
	//nolint:gosec // dummy credentials for tests
	awsCredsContent := "AWS_ACCESS_KEY_ID=testkey\nAWS_SECRET_ACCESS_KEY=testsecret\n"
	awsCredsPath := filepath.Join(tempDir, "aws_credentials")
	if err := os.WriteFile(awsCredsPath, []byte(awsCredsContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Plugins.Cloud.CredentialsPath = awsCredsPath
	cfg.Plugins.Cloud.Region = "us-east-1"

	awsCfg, err := getAWSConfig(context.Background(), cfg, "us-east-1")
	if err != nil {
		t.Fatalf("getAWSConfig failed: %v", err)
	}
	// Verify region
	if awsCfg.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", awsCfg.Region)
	}

	// 2. Test GCP credentials project ID parsing
	//nolint:gosec // dummy credentials for tests
	gcpCredsContent := `{"type": "service_account", "project_id": "gcp-test-project-999"}`
	gcpCredsPath := filepath.Join(tempDir, "gcp_credentials.json")
	if err = os.WriteFile(gcpCredsPath, []byte(gcpCredsContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfgGCP := &config.Config{}
	cfgGCP.Plugins.Cloud.CredentialsPath = gcpCredsPath

	// Clear env var to force fallback to file parsing
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	projectID := getGCPProject(cfgGCP)
	if projectID != "gcp-test-project-999" {
		t.Errorf("expected project ID gcp-test-project-999, got %s", projectID)
	}

	opts, err := getGCPOptions(cfgGCP)
	if err != nil {
		t.Fatalf("getGCPOptions failed: %v", err)
	}
	if len(opts) == 0 {
		t.Errorf("expected client options to contain credentials file option")
	}
}

func TestRealClientsErrorsWithMockHTTP(t *testing.T) {
	// Start mock server that always returns 400 (Bad Request) so clients don't retry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	t.Setenv("POWERWORD_CLOUD_MOCK_ENDPOINT", server.URL)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

	cfg := &config.Config{}
	cfg.Plugins.Cloud.Region = "us-east-1"
	cfg.Plugins.Cloud.Bucket = "test-bucket"

	ctx := context.Background()

	// 1. Test EC2 DescribeInstances error path
	ec2 := &realEC2Client{cfg: cfg}
	_, err := ec2.DescribeInstances(ctx, "us-east-1", nil)
	if err == nil {
		t.Error("expected EC2 client to fail on 500 error, got nil")
	}

	// 2. Test GCE ListInstances error path
	gce := &realGCEClient{cfg: cfg}
	_, err = gce.ListInstances(ctx, "test-project-123", "us-central1-a", nil)
	if err == nil {
		t.Error("expected GCE client to fail on 500 error, got nil")
	}

	// 3. Test CW GetLogEvents error path
	cw := &realCWLogsClient{cfg: cfg}
	_, err = cw.GetLogEvents(ctx, "us-east-1", "group", "stream", 10)
	if err == nil {
		t.Error("expected CW client to fail on 500 error, got nil")
	}

	// 4. Test GCP get log entries error path
	gcpLog := &realGCPLogClient{cfg: cfg}
	_, err = gcpLog.GetLogEntries(ctx, "test-project-123", "stdout", 10)
	if err == nil {
		t.Error("expected GCP logging client to fail on 500 error, got nil")
	}

	// 5. Test S3 bucket error path (HEAD bucket returning 500 means Exists should be false)
	s3c := &realS3Client{cfg: cfg}
	meta, err := s3c.CheckBucket(ctx, "us-east-1", "test-bucket")
	if err != nil {
		t.Fatalf("CheckBucket failed: %v", err)
	}
	if meta.Exists {
		t.Error("expected S3 bucket exists to be false on 500 error")
	}

	// 6. Test GCS bucket error path (Attrs returning 500 means Exists should be false)
	gcsc := &realGCSClient{cfg: cfg}
	gcsMeta, err := gcsc.CheckBucket(ctx, "test-project-123", "test-bucket")
	if err != nil {
		t.Fatalf("CheckBucket failed: %v", err)
	}
	if gcsMeta.Exists {
		t.Error("expected GCS bucket exists to be false on 500 error")
	}
}

func TestCloudServiceCornerCases(t *testing.T) {
	cfg := &config.Config{}
	svc := NewCloudService(cfg, nil, nil, nil, nil, nil, nil)
	ctx := context.Background()

	// 1. GCE ListInstances Missing Project Error
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	_, err := svc.ListInstances(ctx, "gcp", "us-central1-a", "", nil)
	if err == nil || !strings.Contains(err.Error(), "GCP project ID is not configured") {
		t.Errorf("expected missing GCS project ID error, got: %v", err)
	}

	// 2. GCP GetLogs Missing Project Error
	_, err = svc.GetLogs(ctx, "gcp", "", "stdout", "", 10)
	if err == nil || !strings.Contains(err.Error(), "GCP project ID is not configured") {
		t.Errorf("expected missing GCS project ID error, got: %v", err)
	}

	// 3. GCP CheckBucket Missing Project Error
	_, err = svc.CheckBucket(ctx, "gcp", "", "test-bucket")
	if err == nil || !strings.Contains(err.Error(), "GCP project ID is not configured") {
		t.Errorf("expected missing GCS project ID error, got: %v", err)
	}

	// 4. CheckBucket Missing Bucket Name Error
	_, err = svc.CheckBucket(ctx, "aws", "us-east-1", "")
	if err == nil || !strings.Contains(err.Error(), "bucket name is required") {
		t.Errorf("expected bucket name required error, got: %v", err)
	}

	// 5. GetLogs AWS Missing LogGroup Error
	_, err = svc.GetLogs(ctx, "aws", "us-east-1", "", "stream", 10)
	if err == nil || !strings.Contains(err.Error(), "logGroup name is required") {
		t.Errorf("expected logGroup required error, got: %v", err)
	}

	// 6. GetLogs Unsupported Provider Error
	_, err = svc.GetLogs(ctx, "unknown-provider", "", "group", "", 10)
	if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("expected unsupported provider error, got: %v", err)
	}

	// 7. CheckBucket Unsupported Provider Error
	cfg.Plugins.Cloud.Provider = "unknown-provider"
	_, err = svc.CheckBucket(ctx, "", "", "test-bucket")
	if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("expected unsupported provider error, got: %v", err)
	}
}
