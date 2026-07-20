package realimage_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type statusResponse struct {
	Running    bool   `json:"is_running"`
	State      string `json:"state"`
	PID        int    `json:"pid"`
	Generation uint64 `json:"generation"`
}

func (s *scenarioState) apiEventuallyHealthy() error {
	return eventually("management API to become healthy", realImageTimeout, s.apiHealthy)
}

func (s *scenarioState) apiHealthy() error {
	status, body, err := s.httpRequest(http.MethodGet, "/healthz", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("health endpoint returned %d: %s", status, body)
	}
	return nil
}

func (s *scenarioState) serverEventuallyRunning() error {
	return eventually("packaged Bedrock server to report running", realImageTimeout, func() error {
		status, err := s.currentStatus()
		if err != nil {
			return err
		}
		if !status.Running || status.State != "running" || status.Generation == 0 {
			return fmt.Errorf("server status is running=%t state=%q generation=%d", status.Running, status.State, status.Generation)
		}
		return nil
	})
}

func (s *scenarioState) serverEventuallyStopped() error {
	return eventually("packaged Bedrock server to report stopped", realImageTimeout, func() error {
		status, err := s.currentStatus()
		if err != nil {
			return err
		}
		if status.Running || status.State != "stopped" {
			return fmt.Errorf("server status is running=%t state=%q", status.Running, status.State)
		}
		return nil
	})
}

func (s *scenarioState) logsContainExpectedVersion() error {
	return s.logsEventuallyContain("Version: " + s.suite.expectedVersion)
}

func (s *scenarioState) sendCommand(command string) error {
	payload, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return err
	}
	status, body, err := s.httpRequest(http.MethodPost, "/command", payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("command endpoint returned %d: %s", status, body)
	}
	return nil
}

func (s *scenarioState) logsEventuallyContain(expected string) error {
	return eventually("local logs to contain "+expected, realImageTimeout, func() error {
		logs, err := s.currentLogs()
		if err != nil {
			return err
		}
		if !strings.Contains(logs, expected) {
			return fmt.Errorf("local logs do not contain %q", expected)
		}
		return nil
	})
}

func (s *scenarioState) legacyScoreboardStateIsPreserved() error {
	expected := s.legacyScore
	testCommand := fmt.Sprintf(
		"scoreboard players test %s %s %d %d",
		legacyPlayerName,
		legacyObjectiveName,
		expected,
		expected,
	)
	if err := s.sendCommand(testCommand); err != nil {
		return fmt.Errorf("query legacy scoreboard state: %w", err)
	}
	if err := s.logsEventuallyContain(legacyScoreSentence(expected, expected)); err != nil {
		return err
	}

	// Before backup, advance the durable state and confirm the command queue has
	// applied it. The post-backup assertion then searches for a new exact result,
	// so an old log line cannot make a failed restart look successful.
	if s.legacyChecks == 0 {
		next := expected + 1
		if err := s.sendCommand(fmt.Sprintf(
			"scoreboard players set %s %s %d",
			legacyPlayerName,
			legacyObjectiveName,
			next,
		)); err != nil {
			return fmt.Errorf("advance legacy scoreboard state: %w", err)
		}
		if err := s.sendCommand(fmt.Sprintf(
			"scoreboard players test %s %s %d %d",
			legacyPlayerName,
			legacyObjectiveName,
			next,
			next+1,
		)); err != nil {
			return fmt.Errorf("confirm advanced legacy scoreboard state: %w", err)
		}
		if err := s.logsEventuallyContain(legacyScoreSentence(next, next+1)); err != nil {
			return err
		}
		s.legacyScore = next
	}
	s.legacyChecks++
	return nil
}

func (s *scenarioState) requestStopsConcurrently(count int) error {
	if count < 2 || count > 16 {
		return fmt.Errorf("concurrent stop count must be between 2 and 16")
	}
	status, err := s.currentStatus()
	if err != nil {
		return err
	}
	s.initialGeneration = status.Generation
	s.concurrentResults = s.concurrentRequests(http.MethodPost, "/stop", nil, count)
	return nil
}

func (s *scenarioState) requestStartsConcurrently(count int) error {
	if count < 2 || count > 16 {
		return fmt.Errorf("concurrent start count must be between 2 and 16")
	}
	s.concurrentResults = s.concurrentRequests(http.MethodPost, "/start", nil, count)
	return nil
}

func (s *scenarioState) oneLifecycleRequestSucceeds() error {
	if len(s.concurrentResults) < 2 {
		return fmt.Errorf("fewer than two concurrent lifecycle responses were recorded")
	}
	successes := 0
	conflicts := 0
	for index, result := range s.concurrentResults {
		if result.err != nil {
			return fmt.Errorf("lifecycle request %d: %w", index+1, result.err)
		}
		switch result.status {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			return fmt.Errorf("lifecycle request %d returned %d: %s", index+1, result.status, result.body)
		}
	}
	if successes != 1 || conflicts != len(s.concurrentResults)-1 {
		return fmt.Errorf("lifecycle results have %d successes and %d conflicts, want 1 and %d", successes, conflicts, len(s.concurrentResults)-1)
	}
	return nil
}

func (s *scenarioState) processGenerationIncreasesBy(increase int) error {
	if increase < 0 {
		return fmt.Errorf("generation increase must not be negative")
	}
	expected := s.initialGeneration + uint64(increase)
	return eventually(fmt.Sprintf("process generation to become %d", expected), realImageTimeout, func() error {
		status, err := s.currentStatus()
		if err != nil {
			return err
		}
		if status.Generation != expected {
			return fmt.Errorf("process generation is %d", status.Generation)
		}
		return nil
	})
}

func (s *scenarioState) collectorEventuallyContains(expected string) error {
	if s.collector == "" {
		return fmt.Errorf("OpenTelemetry Collector is not running")
	}
	return eventually("Collector logs to contain "+expected, realImageTimeout, func() error {
		logs, err := s.containerLogs(s.collector)
		if err != nil {
			return err
		}
		if !strings.Contains(logs, expected) {
			return fmt.Errorf("Collector logs do not contain %q", expected)
		}
		return nil
	})
}

func (s *scenarioState) collectorContainsServiceIdentity() error {
	if s.collector == "" {
		return fmt.Errorf("OpenTelemetry Collector is not running")
	}
	logs, err := s.containerLogs(s.collector)
	if err != nil {
		return err
	}
	for _, expected := range []string{"service.name", "montainer", "service.instance.id", instanceName, "log.iostream", "stdout"} {
		if !strings.Contains(logs, expected) {
			return fmt.Errorf("Collector export does not contain %q", expected)
		}
	}
	return nil
}

func (s *scenarioState) requestBackupsConcurrently(count int) error {
	if count < 2 || count > 16 {
		return fmt.Errorf("concurrent backup count must be between 2 and 16")
	}
	status, err := s.currentStatus()
	if err != nil {
		return err
	}
	s.initialGeneration = status.Generation
	s.concurrentResults = s.concurrentRequests(http.MethodPost, "/save", nil, count)
	return nil
}

func (s *scenarioState) oneBackupSucceeds() error {
	if len(s.concurrentResults) < 2 {
		return fmt.Errorf("fewer than two backup responses were recorded")
	}
	successes := 0
	conflicts := 0
	for index, result := range s.concurrentResults {
		if result.err != nil {
			return fmt.Errorf("backup request %d: %w", index+1, result.err)
		}
		switch result.status {
		case http.StatusOK:
			successes++
			var response struct {
				Backup backupResult `json:"backup"`
			}
			if err := json.Unmarshal(result.body, &response); err != nil {
				return fmt.Errorf("decode successful backup response: %w: %s", err, result.body)
			}
			s.lastBackup = response.Backup
		case http.StatusConflict:
			conflicts++
		default:
			return fmt.Errorf("backup request %d returned %d: %s", index+1, result.status, result.body)
		}
	}
	if successes != 1 || conflicts != len(s.concurrentResults)-1 {
		return fmt.Errorf("backup results have %d successes and %d conflicts, want 1 and %d", successes, conflicts, len(s.concurrentResults)-1)
	}
	if s.lastBackup.Key == "" || s.lastBackup.Size <= 0 || !s.lastBackup.WasRunning {
		return fmt.Errorf("invalid backup result: %+v", s.lastBackup)
	}
	return nil
}

func (s *scenarioState) uploadedBackupIsValid() error {
	reader, err := s.downloadedBackupZIP()
	if err != nil {
		return err
	}
	names := make(map[string]struct{}, len(reader.File))
	hasWorldData := false
	hasLevelDatabase := false
	for _, file := range reader.File {
		names[file.Name] = struct{}{}
		if strings.HasPrefix(file.Name, "worlds/acceptance-world/") && !file.FileInfo().IsDir() {
			hasWorldData = true
		}
		if strings.HasPrefix(file.Name, "worlds/acceptance-world/db/") && !file.FileInfo().IsDir() {
			hasLevelDatabase = true
		}
		if file.FileInfo().IsDir() {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return fmt.Errorf("open ZIP entry %s: %w", file.Name, err)
		}
		_, copyErr := io.Copy(io.Discard, stream)
		closeErr := stream.Close()
		if copyErr != nil {
			return fmt.Errorf("verify ZIP entry %s: %w", file.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close ZIP entry %s: %w", file.Name, closeErr)
		}
	}
	for _, required := range []string{"server.properties", "allowlist.json", "permissions.json", "worlds/acceptance-world/level.dat"} {
		if _, ok := names[required]; !ok {
			return fmt.Errorf("backup ZIP does not contain %s", required)
		}
	}
	if !hasWorldData || !hasLevelDatabase {
		return fmt.Errorf("backup ZIP does not contain generated world and LevelDB data")
	}
	return nil
}

func (s *scenarioState) uploadedBackupRetainsLegacyWorld() error {
	reader, err := s.downloadedBackupZIP()
	if err != nil {
		return err
	}
	worldPrefix := "worlds/" + legacyWorldName + "/"
	canaryEntry := worldPrefix + legacyCanaryName
	hasLevelData := false
	hasLevelDatabase := false
	hasCanary := false
	for _, file := range reader.File {
		switch {
		case file.Name == worldPrefix+"level.dat" && !file.FileInfo().IsDir():
			hasLevelData = true
		case strings.HasPrefix(file.Name, worldPrefix+"db/") && !file.FileInfo().IsDir() && file.UncompressedSize64 > 0:
			hasLevelDatabase = true
		case file.Name == canaryEntry && !file.FileInfo().IsDir():
			stream, openErr := file.Open()
			if openErr != nil {
				return fmt.Errorf("open legacy canary from backup: %w", openErr)
			}
			contents, readErr := io.ReadAll(io.LimitReader(stream, 4<<10))
			closeErr := stream.Close()
			if readErr != nil {
				return fmt.Errorf("read legacy canary from backup: %w", readErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close legacy canary from backup: %w", closeErr)
			}
			if string(contents) != legacyCanaryContents {
				return fmt.Errorf("legacy canary contents are %q, want %q", contents, legacyCanaryContents)
			}
			hasCanary = true
		}
	}
	if !hasLevelData || !hasLevelDatabase || !hasCanary {
		return fmt.Errorf(
			"backup retained legacy world level.dat=%t LevelDB=%t canary=%t",
			hasLevelData,
			hasLevelDatabase,
			hasCanary,
		)
	}
	return nil
}

func (s *scenarioState) downloadedBackupZIP() (*zip.Reader, error) {
	archive, err := s.downloadedBackupBytes()
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open backup ZIP: %w", err)
	}
	return reader, nil
}

func (s *scenarioState) downloadedBackupBytes() ([]byte, error) {
	if s.minioClient == nil {
		return nil, fmt.Errorf("MinIO client is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := s.minioClient.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.minioBucket),
		Key:    aws.String(s.lastBackup.Key),
	})
	if err != nil {
		return nil, fmt.Errorf("download backup %q: %w", s.lastBackup.Key, err)
	}
	defer response.Body.Close()
	archive, err := io.ReadAll(io.LimitReader(response.Body, 512<<20))
	if err != nil {
		return nil, fmt.Errorf("read backup %q: %w", s.lastBackup.Key, err)
	}
	if int64(len(archive)) != s.lastBackup.Size {
		return nil, fmt.Errorf("downloaded archive size is %d, API reported %d", len(archive), s.lastBackup.Size)
	}
	return archive, nil
}

func (s *scenarioState) candidateHasNoPermissionErrors() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	apiLogs, err := s.currentLogs()
	if err != nil {
		return err
	}
	containerLogs, err := s.containerLogs(s.candidate)
	if err != nil {
		return err
	}
	combined := strings.ToLower(apiLogs + "\n" + containerLogs)
	for _, forbidden := range []string{
		"permission denied",
		"operation not permitted",
		"read-only file system",
		"failed to open leveldb",
		"dbstorage chain is invalid",
	} {
		if strings.Contains(combined, forbidden) {
			return fmt.Errorf("candidate logs contain filesystem failure %q", forbidden)
		}
	}
	return nil
}

func (s *scenarioState) currentStatus() (statusResponse, error) {
	code, body, err := s.httpRequest(http.MethodGet, "/status", nil)
	if err != nil {
		return statusResponse{}, err
	}
	if code != http.StatusOK {
		return statusResponse{}, fmt.Errorf("status endpoint returned %d: %s", code, body)
	}
	var response statusResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return statusResponse{}, fmt.Errorf("decode status response: %w: %s", err, body)
	}
	return response, nil
}

func (s *scenarioState) currentLogs() (string, error) {
	code, body, err := s.httpRequest(http.MethodGet, "/logs?max_lines=2000", nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("logs endpoint returned %d: %s", code, body)
	}
	var response struct {
		Logs []string `json:"logs"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode logs response: %w: %s", err, body)
	}
	return strings.Join(response.Logs, "\n"), nil
}

func (s *scenarioState) concurrentRequests(method, path string, body []byte, count int) []requestResult {
	results := make([]requestResult, count)
	ready := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(count)
	for index := range results {
		go func() {
			defer wait.Done()
			<-ready
			results[index].status, results[index].body, results[index].err = s.httpRequest(method, path, body)
		}()
	}
	close(ready)
	wait.Wait()
	return results
}

func (s *scenarioState) httpRequest(method, path string, body []byte) (int, []byte, error) {
	if s.baseURL == "" {
		return 0, nil, fmt.Errorf("candidate container is not running")
	}
	request, err := http.NewRequest(method, s.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode, responseBody, nil
}

func (s *scenarioState) containerLogs(container string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.docker(ctx, "logs", container)
}

func eventually(description string, timeout time.Duration, assertion func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := assertion(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(realImagePoll)
	}
	return fmt.Errorf("timed out after %s waiting for %s: %w", timeout, description, lastErr)
}
