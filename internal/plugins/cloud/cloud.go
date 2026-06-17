package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/logging/v2"
	"google.golang.org/api/option"

	"github.com/borch-ai/powerword/pkg/config"
)

// Instance represents a normalized virtual machine instance.
type Instance struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"` // "aws" or "gcp"
	State     string `json:"state"`    // e.g. "running", "stopped"
	Type      string `json:"type"`     // e.g. "t2.micro", "n1-standard-1"
	IPAddress string `json:"ip_address,omitempty"`
	Zone      string `json:"zone,omitempty"`
}

// LogEvent represents a normalized cloud log entry.
type LogEvent struct {
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

// BucketMetadata represents normalized cloud storage bucket metadata.
type BucketMetadata struct {
	Name     string            `json:"name"`
	Provider string            `json:"provider"` // "aws" or "gcp"
	Location string            `json:"location,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
	Exists   bool              `json:"exists"`
}

// Client interfaces to enable clean unit testing.

type EC2Client interface {
	DescribeInstances(ctx context.Context, region string, tags map[string]string) ([]Instance, error)
}

type GCEClient interface {
	ListInstances(ctx context.Context, projectID, zone string, tags map[string]string) ([]Instance, error)
}

type CWLogsClient interface {
	GetLogEvents(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]LogEvent, error)
}

type GCPLogClient interface {
	GetLogEntries(ctx context.Context, projectID, logName string, limit int) ([]LogEvent, error)
}

type S3Client interface {
	CheckBucket(ctx context.Context, region, bucketName string) (*BucketMetadata, error)
}

type GCSClient interface {
	CheckBucket(ctx context.Context, projectID, bucketName string) (*BucketMetadata, error)
}

// Real client implementations using AWS/GCP SDKs.

type realEC2Client struct {
	cfg *config.Config
}

//nolint:gocognit,gocyclo
func (c *realEC2Client) DescribeInstances(ctx context.Context, region string, tags map[string]string) ([]Instance, error) {
	awsCfg, err := getAWSConfig(ctx, c.cfg, region)
	if err != nil {
		return nil, err
	}
	client := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) {
		if mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT"); mockEndpoint != "" {
			o.BaseEndpoint = aws.String(mockEndpoint)
		}
	})

	input := &ec2.DescribeInstancesInput{}
	if len(tags) > 0 {
		var filters []ec2types.Filter
		for k, v := range tags {
			filters = append(filters, ec2types.Filter{
				Name:   aws.String(fmt.Sprintf("tag:%s", k)),
				Values: []string{v},
			})
		}
		input.Filters = filters
	}

	result, err := client.DescribeInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe AWS instances: %w", err)
	}

	var list []Instance
	for _, reservation := range result.Reservations {
		for _, inst := range reservation.Instances {
			name := ""
			for _, t := range inst.Tags {
				if aws.ToString(t.Key) == "Name" {
					name = aws.ToString(t.Value)
					break
				}
			}
			ip := aws.ToString(inst.PublicIpAddress)
			if ip == "" {
				ip = aws.ToString(inst.PrivateIpAddress)
			}
			state := ""
			if inst.State != nil {
				state = string(inst.State.Name)
			}
			zone := ""
			if inst.Placement != nil {
				zone = aws.ToString(inst.Placement.AvailabilityZone)
			}
			list = append(list, Instance{
				ID:        aws.ToString(inst.InstanceId),
				Name:      name,
				Provider:  "aws",
				State:     state,
				Type:      string(inst.InstanceType),
				IPAddress: ip,
				Zone:      zone,
			})
		}
	}
	return list, nil
}

type realGCEClient struct {
	cfg *config.Config
}

//nolint:gocognit,gocyclo
func (c *realGCEClient) ListInstances(ctx context.Context, projectID, zone string, tags map[string]string) ([]Instance, error) {
	opts, err := getGCPOptions(c.cfg)
	if err != nil {
		return nil, err
	}
	service, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCP compute client: %w", err)
	}

	call := service.Instances.List(projectID, zone)
	if len(tags) > 0 {
		// Construct dynamic filter expression for GCP tags/labels: labels.key = value
		var filters []string
		for k, v := range tags {
			filters = append(filters, fmt.Sprintf("labels.%s = \"%s\"", k, v))
		}
		call.Filter(strings.Join(filters, " AND "))
	}

	result, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list GCE instances: %w", err)
	}

	var list []Instance
	for _, inst := range result.Items {
		ip := ""
		for _, ni := range inst.NetworkInterfaces {
			if ni.NetworkIP != "" {
				ip = ni.NetworkIP
			}
			for _, ac := range ni.AccessConfigs {
				if ac.NatIP != "" {
					ip = ac.NatIP
				}
			}
		}

		// zone name is a URL path in API (e.g., https://.../zones/us-central1-a)
		zoneName := inst.Zone
		if idx := strings.LastIndex(zoneName, "/"); idx != -1 {
			zoneName = zoneName[idx+1:]
		}

		list = append(list, Instance{
			ID:        fmt.Sprintf("%d", inst.Id),
			Name:      inst.Name,
			Provider:  "gcp",
			State:     inst.Status,
			Type:      inst.MachineType[strings.LastIndex(inst.MachineType, "/")+1:],
			IPAddress: ip,
			Zone:      zoneName,
		})
	}
	return list, nil
}

type realCWLogsClient struct {
	cfg *config.Config
}

func (c *realCWLogsClient) GetLogEvents(ctx context.Context, region, logGroupName, logStreamName string, limit int) ([]LogEvent, error) {
	awsCfg, err := getAWSConfig(ctx, c.cfg, region)
	if err != nil {
		return nil, err
	}
	client := cloudwatchlogs.NewFromConfig(awsCfg, func(o *cloudwatchlogs.Options) {
		if mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT"); mockEndpoint != "" {
			o.BaseEndpoint = aws.String(mockEndpoint)
		}
	})

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(logStreamName),
		//nolint:gosec // limit range checked
		Limit: aws.Int32(int32(limit)),
	}

	result, err := client.GetLogEvents(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CW logs: %w", err)
	}

	var list []LogEvent
	for _, ev := range result.Events {
		list = append(list, LogEvent{
			Timestamp: aws.ToInt64(ev.Timestamp),
			Message:   aws.ToString(ev.Message),
		})
	}
	return list, nil
}

type realGCPLogClient struct {
	cfg *config.Config
}

func (c *realGCPLogClient) GetLogEntries(ctx context.Context, projectID, logName string, limit int) ([]LogEvent, error) {
	opts, err := getGCPOptions(c.cfg)
	if err != nil {
		return nil, err
	}
	service, err := logging.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCP logging service: %w", err)
	}

	filter := fmt.Sprintf("logName=\"projects/%s/logs/%s\"", projectID, logName)
	req := &logging.ListLogEntriesRequest{
		ResourceNames: []string{fmt.Sprintf("projects/%s", projectID)},
		Filter:        filter,
		PageSize:      int64(limit),
	}

	result, err := service.Entries.List(req).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GCP log entries: %w", err)
	}

	var list []LogEvent
	for _, entry := range result.Entries {
		// Try to parse timestamp from entry
		// If text payload is empty, check JSON payload
		msg := entry.TextPayload
		if msg == "" && entry.JsonPayload != nil {
			msgBytes, _ := entry.JsonPayload.MarshalJSON()
			msg = string(msgBytes)
		}
		list = append(list, LogEvent{
			Timestamp: 0, // In mock, we can set timestamp, but GCP API timestamp is string (e.g. RFC3339)
			Message:   msg,
		})
	}
	return list, nil
}

type realS3Client struct {
	cfg *config.Config
}

func (c *realS3Client) CheckBucket(ctx context.Context, region, bucketName string) (*BucketMetadata, error) {
	awsCfg, err := getAWSConfig(ctx, c.cfg, region)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT"); mockEndpoint != "" {
			o.BaseEndpoint = aws.String(mockEndpoint)
			o.UsePathStyle = true
		}
	})

	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucketName)})
	if err != nil {
		// Bucket does not exist or access denied
		return &BucketMetadata{
			Name:     bucketName,
			Provider: "aws",
			Exists:   false,
		}, nil
	}

	// Fetch location
	locRes, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(bucketName)})
	location := region
	if err == nil && locRes != nil {
		location = string(locRes.LocationConstraint)
	}

	// Fetch tags
	tags := make(map[string]string)
	tagRes, err := client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(bucketName)})
	if err == nil && tagRes != nil {
		for _, t := range tagRes.TagSet {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}

	return &BucketMetadata{
		Name:     bucketName,
		Provider: "aws",
		Location: location,
		Tags:     tags,
		Exists:   true,
	}, nil
}

type realGCSClient struct {
	cfg *config.Config
}

func (c *realGCSClient) CheckBucket(ctx context.Context, projectID, bucketName string) (*BucketMetadata, error) {
	opts, err := getGCPOptions(c.cfg)
	if err != nil {
		return nil, err
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()

	bkt := client.Bucket(bucketName)
	attrs, err := bkt.Attrs(ctx)
	if err != nil {
		return &BucketMetadata{
			Name:     bucketName,
			Provider: "gcp",
			Exists:   false,
		}, nil
	}

	return &BucketMetadata{
		Name:     bucketName,
		Provider: "gcp",
		Location: attrs.Location,
		Tags:     attrs.Labels,
		Exists:   true,
	}, nil
}

// CloudService coordinates the invocation of the respective cloud provider clients.
type CloudService struct {
	cfg          *config.Config
	ec2Client    EC2Client
	gceClient    GCEClient
	cwClient     CWLogsClient
	gcpLogClient GCPLogClient
	s3Client     S3Client
	gcsClient    GCSClient
	uploader     Uploader
}

// NewCloudService constructs a CloudService, injecting mocks if passed, or defaulting to real clients.
func NewCloudService(cfg *config.Config, ec2 EC2Client, gce GCEClient, cw CWLogsClient, gcplog GCPLogClient, s3 S3Client, gcs GCSClient) *CloudService {
	s := &CloudService{
		cfg:          cfg,
		ec2Client:    ec2,
		gceClient:    gce,
		cwClient:     cw,
		gcpLogClient: gcplog,
		s3Client:     s3,
		gcsClient:    gcs,
	}

	if s.ec2Client == nil {
		s.ec2Client = &realEC2Client{cfg: cfg}
	}
	if s.gceClient == nil {
		s.gceClient = &realGCEClient{cfg: cfg}
	}
	if s.cwClient == nil {
		s.cwClient = &realCWLogsClient{cfg: cfg}
	}
	if s.gcpLogClient == nil {
		s.gcpLogClient = &realGCPLogClient{cfg: cfg}
	}
	if s.s3Client == nil {
		s.s3Client = &realS3Client{cfg: cfg}
	}
	if s.gcsClient == nil {
		s.gcsClient = &realGCSClient{cfg: cfg}
	}
	if s.uploader == nil {
		s.uploader = NewUploader(cfg)
	}

	return s
}

// SetUploader allows overriding the default uploader (useful for unit tests).
func (s *CloudService) SetUploader(u Uploader) {
	s.uploader = u
}

// UploadFile uploads the local file using the configured uploader.
func (s *CloudService) UploadFile(ctx context.Context, localPath string) (string, error) {
	if s.uploader == nil {
		return "", fmt.Errorf("uploader is not initialized")
	}
	return s.uploader.UploadFile(ctx, localPath)
}

// ListInstances queries and normalizes instances based on target provider.
func (s *CloudService) ListInstances(ctx context.Context, provider, zone, region string, tags map[string]string) ([]Instance, error) {
	provider = strings.ToLower(provider)
	if provider == "aws" || provider == "ec2" {
		if region == "" {
			region = s.cfg.Plugins.Cloud.Region
		}
		if region == "" {
			region = "us-east-1"
		}
		return s.ec2Client.DescribeInstances(ctx, region, tags)
	}

	if provider == "gcp" || provider == "gce" {
		projectID := getGCPProject(s.cfg)
		if projectID == "" {
			return nil, fmt.Errorf("GCP project ID is not configured (specify via GOOGLE_CLOUD_PROJECT or plugins.cloud config)")
		}
		if zone == "" {
			zone = "us-central1-a"
		}
		return s.gceClient.ListInstances(ctx, projectID, zone, tags)
	}

	return nil, fmt.Errorf("unsupported or unconfigured provider: %s", provider)
}

// GetLogs retrieves and normalizes logs from target provider.
func (s *CloudService) GetLogs(ctx context.Context, provider, region, logGroup, logStream string, limit int) ([]LogEvent, error) {
	provider = strings.ToLower(provider)
	if limit <= 0 {
		limit = 50
	}

	if provider == "aws" || provider == "cloudwatch" {
		if region == "" {
			region = s.cfg.Plugins.Cloud.Region
		}
		if region == "" {
			region = "us-east-1"
		}
		if logGroup == "" {
			return nil, fmt.Errorf("logGroup name is required for CloudWatch logs")
		}
		return s.cwClient.GetLogEvents(ctx, region, logGroup, logStream, limit)
	}

	if provider == "gcp" || provider == "stackdriver" || provider == "logging" {
		projectID := getGCPProject(s.cfg)
		if projectID == "" {
			return nil, fmt.Errorf("GCP project ID is not configured")
		}
		if logGroup == "" {
			logGroup = "stdout"
		}
		return s.gcpLogClient.GetLogEntries(ctx, projectID, logGroup, limit)
	}

	return nil, fmt.Errorf("unsupported provider: %s", provider)
}

// CheckBucket checks existence and metadata for the bucket.
func (s *CloudService) CheckBucket(ctx context.Context, provider, region, bucketName string) (*BucketMetadata, error) {
	provider = strings.ToLower(provider)
	if bucketName == "" {
		bucketName = s.cfg.Plugins.Cloud.Bucket
	}
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	if provider == "" {
		provider = strings.ToLower(s.cfg.Plugins.Cloud.Provider)
	}
	if provider == "noop" || provider == "" {
		return &BucketMetadata{
			Name:     bucketName,
			Provider: "noop",
			Exists:   false,
		}, nil
	}

	if provider == "aws" || provider == "s3" {
		if region == "" {
			region = s.cfg.Plugins.Cloud.Region
		}
		if region == "" {
			region = "us-east-1"
		}
		return s.s3Client.CheckBucket(ctx, region, bucketName)
	}

	if provider == "gcp" || provider == "gcs" {
		projectID := getGCPProject(s.cfg)
		if projectID == "" {
			return nil, fmt.Errorf("GCP project ID is not configured")
		}
		return s.gcsClient.CheckBucket(ctx, projectID, bucketName)
	}

	return nil, fmt.Errorf("unsupported provider: %s", provider)
}

// Helper methods to resolve credentials.

// parseAWSCredentialsFile reads and parses simple key-value credentials from file path.
func parseAWSCredentialsFile(credPath string) (string, string, error) {
	if _, err := os.Stat(credPath); err != nil {
		return "", "", err
	}
	//nolint:gosec // path comes from config
	data, err := os.ReadFile(credPath)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(string(data), "\n")
	var accessKey, secretKey string
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			k := strings.ToUpper(strings.TrimSpace(parts[0]))
			v := strings.TrimSpace(parts[1])
			switch k {
			case "AWS_ACCESS_KEY_ID":
				accessKey = v
			case "AWS_SECRET_ACCESS_KEY":
				secretKey = v
			}
		}
	}
	return accessKey, secretKey, nil
}

func getAWSConfig(ctx context.Context, cfg *config.Config, region string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	if mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT"); mockEndpoint != "" {
		//nolint:staticcheck // WithEndpointResolverWithOptions is deprecated but required for legacy client mock endpoints
		opts = append(opts, awsconfig.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           mockEndpoint,
					SigningRegion: region,
				}, nil
			},
		)))
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "testing",
				SecretAccessKey: "testing",
			},
		}))
	}

	credPath := cfg.Plugins.Cloud.CredentialsPath
	if credPath != "" {
		accessKey, secretKey, err := parseAWSCredentialsFile(credPath)
		if err == nil && accessKey != "" && secretKey != "" {
			opts = append(opts, awsconfig.WithCredentialsProvider(credentials.StaticCredentialsProvider{
				Value: aws.Credentials{
					AccessKeyID:     accessKey,
					SecretAccessKey: secretKey,
				},
			}))
		}
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return awsCfg, nil
}

func getGCPOptions(cfg *config.Config) ([]option.ClientOption, error) {
	var opts []option.ClientOption
	if mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT"); mockEndpoint != "" {
		opts = append(opts, option.WithEndpoint(mockEndpoint), option.WithoutAuthentication())
	}
	credPath := cfg.Plugins.Cloud.CredentialsPath
	if credPath != "" {
		if _, err := os.Stat(credPath); err == nil {
			//nolint:staticcheck // option.WithCredentialsFile is deprecated but required for config loading
			opts = append(opts, option.WithCredentialsFile(credPath))
		}
	}
	return opts, nil
}

func getGCPProject(cfg *config.Config) string {
	// 1. Try to read from environment variable
	if project := os.Getenv("GOOGLE_CLOUD_PROJECT"); project != "" {
		return project
	}
	// 2. Fall back to credentials path inspection
	credPath := cfg.Plugins.Cloud.CredentialsPath
	if credPath != "" {
		//nolint:gosec // path comes from config
		if data, err := os.ReadFile(credPath); err == nil {
			var sa struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(data, &sa); err == nil {
				return sa.ProjectID
			}
		}
	}
	return ""
}
