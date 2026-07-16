package realimage_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	minioAccessKey = "montainer-access"
	minioSecretKey = "montainer-secret-acceptance"
)

func (s *scenarioState) docker(ctx context.Context, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "docker", arguments...)
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("docker %s: %w\n%s", strings.Join(arguments, " "), err, text)
	}
	return text, nil
}

func (s *scenarioState) dockerWithin(timeout time.Duration, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.docker(ctx, arguments...)
}

func (s *scenarioState) candidateImageAvailable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.docker(ctx, "image", "inspect", s.suite.image)
	return err
}

func (s *scenarioState) startCollector() error {
	if s.candidate != "" {
		return fmt.Errorf("Collector must be configured before the candidate starts")
	}
	name := strings.Replace(s.networkName, "montainer-real-", "montainer-otel-", 1)
	s.collector = name
	s.containers = append(s.containers, name)
	configPath := filepath.Join(s.suite.repoRoot, "examples", "docker", "otel-collector.yaml")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.docker(
		ctx,
		"run", "--detach",
		"--name", name,
		"--network", s.networkName,
		"--volume", configPath+":/etc/otelcol-contrib/config.yaml:ro",
		s.suite.collectorImage,
		"--config=/etc/otelcol-contrib/config.yaml",
	); err != nil {
		return err
	}
	if err := eventually("OpenTelemetry Collector to start", 45*time.Second, func() error {
		output, err := s.dockerWithin(5*time.Second, "inspect", "--format", "{{.State.Running}}", name)
		if err != nil {
			return err
		}
		if output != "true" {
			return fmt.Errorf("Collector running state is %q", output)
		}
		return nil
	}); err != nil {
		return err
	}
	s.env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://" + name + ":4318"
	return nil
}

func (s *scenarioState) configureUnavailableCollector() error {
	if s.candidate != "" {
		return fmt.Errorf("Collector must be configured before the candidate starts")
	}
	s.env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://missing-collector:4318"
	s.env["OTEL_EXPORTER_OTLP_LOGS_TIMEOUT"] = "250"
	return nil
}

func (s *scenarioState) delayOTelUntilShutdown() error {
	if s.candidate != "" {
		return fmt.Errorf("log batching must be configured before the candidate starts")
	}
	s.env["OTEL_BLRP_SCHEDULE_DELAY"] = "60000"
	s.env["OTEL_BLRP_EXPORT_TIMEOUT"] = "5000"
	return nil
}

func (s *scenarioState) startMinIO() error {
	if s.candidate != "" {
		return fmt.Errorf("MinIO must be configured before the candidate starts")
	}
	name := strings.Replace(s.networkName, "montainer-real-", "montainer-minio-", 1)
	s.minio = name
	s.containers = append(s.containers, name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.docker(
		ctx,
		"run", "--detach",
		"--name", name,
		"--network", s.networkName,
		"--publish", "127.0.0.1::9000/tcp",
		"--env", "MINIO_ROOT_USER="+minioAccessKey,
		"--env", "MINIO_ROOT_PASSWORD="+minioSecretKey,
		s.suite.minioImage,
		"server", "/data", "--console-address", ":9001",
	); err != nil {
		return err
	}
	address, err := s.publishedAddress(ctx, name, "9000/tcp")
	if err != nil {
		return err
	}
	s.minioEndpoint = "http://" + address
	if err := eventually("MinIO to become ready", 60*time.Second, func() error {
		request, err := http.NewRequest(http.MethodGet, s.minioEndpoint+"/minio/health/ready", nil)
		if err != nil {
			return err
		}
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("MinIO readiness returned %d", response.StatusCode)
		}
		return nil
	}); err != nil {
		return err
	}

	awsConfig, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(minioAccessKey, minioSecretKey, "")),
	)
	if err != nil {
		return fmt.Errorf("configure MinIO client: %w", err)
	}
	s.minioClient = s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(s.minioEndpoint)
		options.UsePathStyle = true
	})
	if _, err := s.minioClient.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.minioBucket)}); err != nil {
		return fmt.Errorf("create MinIO acceptance bucket: %w", err)
	}

	s.env["AWS_S3_ENDPOINT"] = "http://" + name + ":9000"
	s.env["AWS_S3_KEY_ID"] = minioAccessKey
	s.env["AWS_S3_SECRET_KEY"] = minioSecretKey
	s.env["AWS_S3_BUCKET_NAME"] = s.minioBucket
	s.env["AWS_S3_REGION"] = "us-east-1"
	return nil
}

func (s *scenarioState) startCandidate() error {
	if s.candidate != "" {
		return fmt.Errorf("candidate container has already started")
	}
	name := strings.Replace(s.networkName, "montainer-real-", "montainer-app-", 1)
	s.candidate = name
	s.containers = append(s.containers, name)
	arguments := []string{
		"run", "--detach",
		"--name", name,
		"--network", s.networkName,
		"--add-host", "host.docker.internal:host-gateway",
		"--publish", "127.0.0.1::8000/tcp",
		"--publish", "127.0.0.1::19132/udp",
		"--volume", s.configDir + ":/app/configs",
	}
	keys := make([]string, 0, len(s.env))
	for key := range s.env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		arguments = append(arguments, "--env", key+"="+s.env[key])
	}
	arguments = append(arguments, s.suite.image)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.docker(ctx, arguments...); err != nil {
		return err
	}
	httpAddress, err := s.publishedAddress(ctx, name, "8000/tcp")
	if err != nil {
		return err
	}
	udpAddress, err := s.publishedAddress(ctx, name, "19132/udp")
	if err != nil {
		return err
	}
	s.baseURL = "http://" + httpAddress
	s.udpAddress = udpAddress
	return nil
}

func (s *scenarioState) stopCandidate() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err := s.docker(ctx, "stop", "--time", "45", s.candidate)
	return err
}

func (s *scenarioState) candidateExitedCleanly() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container has not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := s.docker(ctx, "inspect", "--format", "{{.State.Status}} {{.State.ExitCode}}", s.candidate)
	if err != nil {
		return err
	}
	if state != "exited 0" {
		return fmt.Errorf("candidate container state is %q, want exited 0", state)
	}
	return nil
}

func (s *scenarioState) startVirtualClient() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	if s.suite.clientBinary == "" {
		return fmt.Errorf("Bedrock client probe was not built for this acceptance shard")
	}
	if s.client != "" {
		return fmt.Errorf("virtual Bedrock player has already started")
	}
	name := strings.Replace(s.networkName, "montainer-real-", "montainer-client-", 1)
	s.client = name
	s.containers = append(s.containers, name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.docker(
		ctx,
		"run", "--detach",
		"--name", name,
		"--network", s.networkName,
		"--entrypoint", "/tmp/bedrock-client",
		"--volume", s.suite.clientBinary+":/tmp/bedrock-client:ro",
		s.suite.image,
		"--address", s.candidate+":19132",
		"--username", "MontainerCI",
		"--wait-teleport",
		"--expect-x", "12000",
		"--expect-y", "100",
		"--expect-z", "-12000",
		"--position-tolerance", "3",
		"--timeout", "90s",
	); err != nil {
		return err
	}
	return eventually("virtual Bedrock player to spawn", realImageTimeout, func() error {
		logs, err := s.containerLogs(name)
		if err != nil {
			return err
		}
		if strings.Contains(logs, "spawned player=") {
			return nil
		}
		state, inspectErr := s.dockerWithin(5*time.Second, "inspect", "--format", "{{.State.Status}} {{.State.ExitCode}}", name)
		if inspectErr == nil && strings.HasPrefix(state, "exited ") {
			return fmt.Errorf("virtual player exited before spawning (%s): %s", state, logs)
		}
		return fmt.Errorf("virtual player has not spawned yet: %s", logs)
	})
}

func (s *scenarioState) virtualClientReceivesTeleport() error {
	if s.client == "" {
		return fmt.Errorf("virtual Bedrock player is not running")
	}
	return eventually("virtual Bedrock player to receive a teleport", realImageTimeout, func() error {
		logs, err := s.containerLogs(s.client)
		if err != nil {
			return err
		}
		if !strings.Contains(logs, "teleported runtime_id=") {
			return fmt.Errorf("virtual player has not reported a teleport: %s", logs)
		}
		state, err := s.dockerWithin(5*time.Second, "inspect", "--format", "{{.State.Status}} {{.State.ExitCode}}", s.client)
		if err != nil {
			return err
		}
		if state != "exited 0" {
			return fmt.Errorf("virtual player state is %q, want exited 0", state)
		}
		return nil
	})
}

func (s *scenarioState) publishedAddress(ctx context.Context, container, port string) (string, error) {
	output, err := s.docker(ctx, "port", container, port)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(strings.Split(output, "\n")[0])
	host, mappedPort, err := net.SplitHostPort(line)
	if err != nil {
		return "", fmt.Errorf("parse Docker port %s for %s from %q: %w", port, container, line, err)
	}
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, mappedPort), nil
}
