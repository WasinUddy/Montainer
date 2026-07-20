package realimage_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	minioAccessKey         = "montainer-access"
	minioSecretKey         = "montainer-secret-acceptance"
	legacyWorldName        = "acceptance-world"
	legacyObjectiveName    = "legacy_probe"
	legacyPlayerName       = "LegacyMarker"
	legacyCanaryName       = "montainer-legacy-canary.txt"
	legacyCanaryContents   = "Montainer legacy world acceptance canary\n"
	nestedMountDirectory   = "nested[1]"
	nestedParentCanary     = "parent-must-migrate.txt"
	nestedMountCanary      = "nested-must-stay-root.txt"
	acceptanceVolumeLabel  = "io.montainer.acceptance=real-image"
	containerInstanceRoot  = "/app/instance"
	containerWorldsRoot    = containerInstanceRoot + "/worlds"
	containerCustomRoot    = "/custom-instance"
	containerConfigsRoot   = "/app/configs"
	containerResourcesRoot = "/app/resource_packs"
	containerLogsRoot      = "/app/logs"
)

type volumeMount struct {
	name        string
	destination string
}

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

func (s *scenarioState) seedRootOwnedLegacyWorld() error {
	if s.candidate != "" {
		return fmt.Errorf("legacy world must be seeded before the candidate starts")
	}
	if len(s.legacyMounts) != 0 {
		return fmt.Errorf("legacy named volumes have already been created")
	}
	if err := s.ensureLegacyImageAvailable(); err != nil {
		return err
	}

	s.legacyMounts = []volumeMount{
		{name: s.networkName + "-worlds", destination: containerWorldsRoot},
		{name: s.networkName + "-configs", destination: containerConfigsRoot},
		{name: s.networkName + "-resources", destination: containerResourcesRoot},
		{name: s.networkName + "-logs", destination: containerLogsRoot},
	}
	if err := s.createAcceptanceVolumes(s.legacyMounts); err != nil {
		return err
	}

	configVolume, err := s.legacyVolumeFor(containerConfigsRoot)
	if err != nil {
		return err
	}
	resourceVolume, err := s.legacyVolumeFor(containerResourcesRoot)
	if err != nil {
		return err
	}
	logVolume, err := s.legacyVolumeFor(containerLogsRoot)
	if err != nil {
		return err
	}
	helperName := s.trackHelperContainer("legacy-config-seed")
	if _, err := s.dockerWithin(
		60*time.Second,
		"run", "--name", helperName,
		"--platform", "linux/amd64",
		"--user", "0:0",
		"--entrypoint", "/bin/sh",
		"--volume", configVolume+":"+containerConfigsRoot,
		"--volume", resourceVolume+":"+containerResourcesRoot,
		"--volume", logVolume+":"+containerLogsRoot,
		"--volume", s.configDir+":/tmp/montainer-config-fixtures:ro",
		s.suite.legacyImage,
		"-ceu",
		`for file in server.properties allowlist.json permissions.json; do
			cp "/tmp/montainer-config-fixtures/$file" "/app/configs/$file"
			chown 0:0 "/app/configs/$file"
		done
		printf 'legacy resource pack fixture\n' > /app/resource_packs/legacy-pack.txt
		printf 'legacy log fixture\n' > /app/logs/legacy.log
		chown 0:0 /app/resource_packs/legacy-pack.txt /app/logs/legacy.log`,
	); err != nil {
		return fmt.Errorf("seed root-owned legacy configuration: %w", err)
	}

	legacyName := strings.Replace(s.networkName, "montainer-real-", "montainer-legacy-", 1)
	if err := s.startCandidateContainer(legacyName, s.suite.legacyImage); err != nil {
		return err
	}
	if err := s.apiEventuallyHealthy(); err != nil {
		return fmt.Errorf("wait for pre-v3 Montainer API: %w", err)
	}
	if err := s.rakNetEventuallyResponds(); err != nil {
		return fmt.Errorf("discover pre-v3 Bedrock server: %w", err)
	}
	legacyUID, err := s.dockerWithin(10*time.Second, "exec", s.candidate, "id", "-u")
	if err != nil {
		return fmt.Errorf("inspect pre-v3 runtime identity: %w", err)
	}
	if legacyUID != "0" {
		return fmt.Errorf("pre-v3 runtime UID is %s, want 0", legacyUID)
	}
	for _, command := range []string{
		"scoreboard objectives add " + legacyObjectiveName + " dummy LegacyUpgradeProbe",
		fmt.Sprintf("scoreboard players set %s %s %d", legacyPlayerName, legacyObjectiveName, s.legacyScore),
		fmt.Sprintf("scoreboard players test %s %s %d %d", legacyPlayerName, legacyObjectiveName, s.legacyScore, s.legacyScore),
	} {
		if err := s.sendCommand(command); err != nil {
			return fmt.Errorf("seed pre-v3 scoreboard state with %q: %w", command, err)
		}
	}
	if err := s.logsEventuallyContain(legacyScoreSentence(s.legacyScore, s.legacyScore)); err != nil {
		return fmt.Errorf("verify pre-v3 scoreboard state: %w", err)
	}
	if err := s.stopLegacyServerAndContainer(); err != nil {
		return err
	}
	if err := s.createLegacyCanaryAndVerifyRootOwnership(); err != nil {
		return err
	}

	// The next candidate must reuse these volumes through the image's normal
	// entrypoint so the upgrade path, rather than the seeding process, owns the
	// migration from root to the unprivileged runtime identity.
	s.candidate = ""
	s.baseURL = ""
	s.udpAddress = ""
	return nil
}

func (s *scenarioState) rootOwnedCustomInstanceExists() error {
	if s.candidate != "" {
		return fmt.Errorf("custom instance must be prepared before the candidate starts")
	}
	if len(s.legacyMounts) != 0 {
		return fmt.Errorf("custom instance volumes have already been created")
	}
	if err := s.ensureLegacyImageAvailable(); err != nil {
		return err
	}

	s.legacyMounts = []volumeMount{
		{name: s.networkName + "-custom-instance", destination: containerCustomRoot},
		{name: s.networkName + "-custom-configs", destination: containerConfigsRoot},
		{name: s.networkName + "-custom-resources", destination: containerResourcesRoot},
		{name: s.networkName + "-custom-logs", destination: containerLogsRoot},
	}
	if err := s.createAcceptanceVolumes(s.legacyMounts); err != nil {
		return err
	}

	arguments := []string{
		"run", "--name", s.trackHelperContainer("custom-instance-seed"),
		"--platform", "linux/amd64",
		"--user", "0:0",
		"--entrypoint", "/bin/sh",
	}
	for _, mount := range s.legacyMounts {
		arguments = append(arguments, "--volume", mount.name+":"+mount.destination)
	}
	arguments = append(
		arguments,
		"--volume", s.configDir+":/tmp/montainer-config-fixtures:ro",
		s.suite.legacyImage,
		"-ceu",
		`cp -a /app/instance/. "$1/"
		for file in server.properties allowlist.json permissions.json; do
			cp "/tmp/montainer-config-fixtures/$file" "$2/$file"
		done
		printf 'legacy custom resource fixture\n' > "$3/legacy-custom-pack.txt"
		printf 'legacy custom log fixture\n' > "$4/legacy-custom.log"
		chown -R 0:0 "$1" "$2" "$3" "$4"`,
		"sh",
		containerCustomRoot,
		containerConfigsRoot,
		containerResourcesRoot,
		containerLogsRoot,
	)
	if _, err := s.dockerWithin(2*time.Minute, arguments...); err != nil {
		return fmt.Errorf("prepare root-owned custom pre-v3 instance: %w", err)
	}

	s.env["INSTANCE_DIR"] = containerCustomRoot
	return nil
}

func (s *scenarioState) accessibleRootOwnedNestedMountExists() error {
	if s.candidate != "" {
		return fmt.Errorf("nested mount must be prepared before the candidate starts")
	}
	if len(s.legacyMounts) != 0 {
		return fmt.Errorf("nested-mount volumes have already been created")
	}
	nestedDestination := containerWorldsRoot + "/" + nestedMountDirectory
	s.legacyMounts = []volumeMount{
		{name: s.networkName + "-nested-parent", destination: containerWorldsRoot},
		{name: s.networkName + "-nested-child", destination: nestedDestination},
		{name: s.networkName + "-nested-configs", destination: containerConfigsRoot},
		{name: s.networkName + "-nested-resources", destination: containerResourcesRoot},
		{name: s.networkName + "-nested-logs", destination: containerLogsRoot},
	}
	if err := s.createAcceptanceVolumes(s.legacyMounts); err != nil {
		return err
	}

	arguments := []string{
		"run", "--name", s.trackHelperContainer("nested-mount-seed"),
		"--platform", "linux/amd64",
		"--user", "0:0",
		"--entrypoint", "/bin/sh",
	}
	for _, mount := range s.legacyMounts {
		arguments = append(arguments, "--volume", mount.name+":"+mount.destination)
	}
	arguments = append(
		arguments,
		s.suite.image,
		"-ceu",
		`printf 'parent\n' > "$1/$3"
		printf 'nested\n' > "$2/$4"
		chown -R 0:0 "$1" "$2" "$5" "$6" "$7"
		chmod 0777 "$2"
		chmod 0666 "$2/$4"`,
		"sh",
		containerWorldsRoot,
		nestedDestination,
		nestedParentCanary,
		nestedMountCanary,
		containerConfigsRoot,
		containerResourcesRoot,
		containerLogsRoot,
	)
	if _, err := s.dockerWithin(60*time.Second, arguments...); err != nil {
		return fmt.Errorf("prepare accessible root-owned nested mount: %w", err)
	}
	s.env["BEDROCK_AUTO_START"] = "false"
	return nil
}

func (s *scenarioState) createAcceptanceVolumes(mounts []volumeMount) error {
	for _, mount := range mounts {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := s.docker(
			ctx,
			"volume", "create",
			"--label", acceptanceVolumeLabel,
			"--label", "io.montainer.acceptance.network="+s.networkName,
			mount.name,
		)
		cancel()
		if err != nil {
			return err
		}
		s.volumes = append(s.volumes, mount.name)
	}
	return nil
}

func (s *scenarioState) trackHelperContainer(role string) string {
	suffix := strings.TrimPrefix(s.networkName, "montainer-real-")
	name := "montainer-" + role + "-" + suffix
	s.containers = append(s.containers, name)
	return name
}

func (s *scenarioState) ensureLegacyImageAvailable() error {
	if _, err := s.dockerWithin(30*time.Second, "image", "inspect", s.suite.legacyImage); err == nil {
		return nil
	}
	if _, err := s.dockerWithin(5*time.Minute, "pull", "--platform", "linux/amd64", s.suite.legacyImage); err != nil {
		return fmt.Errorf("pull digest-pinned pre-v3 fixture: %w", err)
	}
	return nil
}

func (s *scenarioState) legacyVolumeFor(destination string) (string, error) {
	for _, mount := range s.legacyMounts {
		if mount.destination == destination {
			return mount.name, nil
		}
	}
	return "", fmt.Errorf("legacy volume for %s is not configured", destination)
}

func (s *scenarioState) stopLegacyServerAndContainer() error {
	status, body, err := s.httpRequest(http.MethodPost, "/stop", nil)
	if err != nil {
		return fmt.Errorf("stop pre-v3 Bedrock server: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("pre-v3 stop endpoint returned %d: %s", status, body)
	}
	if err := eventually("pre-v3 Bedrock process to stop", 60*time.Second, func() error {
		_, err := s.dockerWithin(
			5*time.Second,
			"exec", s.candidate,
			"/bin/sh", "-ceu",
			`for status in /proc/[0-9]*/status; do
				[ -r "$status" ] || continue
				[ "$(awk '$1 == "Name:" { print $2 }' "$status")" != bedrock_server ] ||
					[ "$(awk '$1 == "State:" { print $2 }' "$status")" = Z ] || exit 1
			done`,
		)
		return err
	}); err != nil {
		return err
	}
	if err := s.stopCandidate(); err != nil {
		return fmt.Errorf("stop pre-v3 container: %w", err)
	}
	if err := s.candidateExitedCleanly(); err != nil {
		return fmt.Errorf("pre-v3 container did not exit cleanly: %w", err)
	}
	return nil
}

func (s *scenarioState) createLegacyCanaryAndVerifyRootOwnership() error {
	arguments := []string{
		"run", "--name", s.trackHelperContainer("legacy-volume-inspect"),
		"--platform", "linux/amd64",
		"--user", "0:0",
		"--entrypoint", "/bin/sh",
	}
	for _, mount := range s.legacyMounts {
		arguments = append(arguments, "--volume", mount.name+":"+mount.destination)
	}
	arguments = append(
		arguments,
		s.suite.image,
		"-ceu",
		`world=$1
		canary="$world/$2"
		test -s "$world/level.dat"
		database_file=$(find "$world/db" -type f -size +0c -print -quit)
		test -n "$database_file"
		printf '%s' "$3" > "$canary"
		chown 0:0 "$canary"
		for root in "$4" "$5" "$6" "$7"; do
			unexpected=$(find "$root" -xdev \( -type d -o -type f -o -type l \) \( ! -uid 0 -o ! -gid 0 \) -print -quit)
			test -z "$unexpected" || {
				printf 'non-root legacy entry: %s\n' "$unexpected" >&2
				exit 1
			}
		done`,
		"sh",
		containerWorldsRoot+"/"+legacyWorldName,
		legacyCanaryName,
		legacyCanaryContents,
		containerWorldsRoot,
		containerConfigsRoot,
		containerResourcesRoot,
		containerLogsRoot,
	)
	if _, err := s.dockerWithin(60*time.Second, arguments...); err != nil {
		return fmt.Errorf("verify genuine root-owned pre-v3 persistence: %w", err)
	}
	return nil
}

func legacyScoreSentence(lower, upper int) string {
	return fmt.Sprintf("Score %d is in range %d to %d", lower, lower, upper)
}

func (s *scenarioState) startCandidate() error {
	if s.candidate != "" {
		return fmt.Errorf("candidate container has already started")
	}
	name := strings.Replace(s.networkName, "montainer-real-", "montainer-app-", 1)
	return s.startCandidateContainer(name, s.suite.image)
}

func (s *scenarioState) configureExplicitNonRootCandidate() error {
	if s.candidate != "" {
		return fmt.Errorf("explicit non-root mode must be configured before the candidate starts")
	}
	s.explicitNonRoot = true
	return nil
}

func (s *scenarioState) startCandidateContainer(name, image string) error {
	s.candidate = name
	s.containers = append(s.containers, name)
	arguments := []string{
		"run", "--detach",
		"--platform", "linux/amd64",
		"--name", name,
		"--network", s.networkName,
		"--add-host", "host.docker.internal:host-gateway",
		"--publish", "127.0.0.1::8000/tcp",
		"--publish", "127.0.0.1::19132/udp",
	}
	if s.explicitNonRoot {
		arguments = append(
			arguments,
			"--user", "10001:10001",
			"--cap-drop", "ALL",
			"--security-opt", "no-new-privileges:true",
		)
	}
	if len(s.legacyMounts) == 0 {
		arguments = append(arguments, "--volume", s.configDir+":"+containerConfigsRoot)
	} else {
		for _, mount := range s.legacyMounts {
			arguments = append(arguments, "--volume", mount.name+":"+mount.destination)
		}
	}
	keys := make([]string, 0, len(s.env))
	for key := range s.env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		arguments = append(arguments, "--env", key+"="+s.env[key])
	}
	arguments = append(arguments, image)

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

func (s *scenarioState) restoreUploadedBackupIntoFreshNamedVolumes() error {
	archive, err := s.downloadedBackupBytes()
	if err != nil {
		return err
	}
	archivePath := filepath.Join(s.tmpDir, "legacy-backup.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		return fmt.Errorf("write downloaded backup for restore: %w", err)
	}

	if err := s.stopCandidate(); err != nil {
		return fmt.Errorf("stop upgraded candidate before restore verification: %w", err)
	}
	if err := s.candidateExitedCleanly(); err != nil {
		return fmt.Errorf("upgraded candidate did not exit cleanly before restore verification: %w", err)
	}

	restoredMounts := []volumeMount{
		{name: s.networkName + "-restore-worlds", destination: containerWorldsRoot},
		{name: s.networkName + "-restore-configs", destination: containerConfigsRoot},
		{name: s.networkName + "-restore-resources", destination: containerResourcesRoot},
		{name: s.networkName + "-restore-logs", destination: containerLogsRoot},
	}
	if err := s.createAcceptanceVolumes(restoredMounts); err != nil {
		return err
	}

	worldVolume := ""
	configVolume := ""
	for _, mount := range restoredMounts {
		switch mount.destination {
		case containerWorldsRoot:
			worldVolume = mount.name
		case containerConfigsRoot:
			configVolume = mount.name
		}
	}
	if worldVolume == "" || configVolume == "" {
		return fmt.Errorf("restored world and configuration volumes were not configured")
	}

	const restoreScript = `
import pathlib
import shutil
import stat
import sys
import zipfile

archive_path, worlds_root, configs_root = sys.argv[1:]
config_names = {"server.properties", "allowlist.json", "permissions.json"}

with zipfile.ZipFile(archive_path) as archive:
    for info in archive.infolist():
        source_path = pathlib.PurePosixPath(info.filename)
        if source_path.is_absolute() or ".." in source_path.parts:
            raise RuntimeError(f"unsafe backup path: {info.filename!r}")
        file_type = (info.external_attr >> 16) & 0o170000
        if file_type == stat.S_IFLNK:
            raise RuntimeError(f"refusing backup symlink: {info.filename!r}")

        destination = None
        if len(source_path.parts) > 1 and source_path.parts[0] == "worlds":
            destination = pathlib.Path(worlds_root).joinpath(*source_path.parts[1:])
        elif len(source_path.parts) == 1 and source_path.name in config_names:
            destination = pathlib.Path(configs_root, source_path.name)
        if destination is None:
            continue

        if info.is_dir():
            destination.mkdir(parents=True, exist_ok=True)
            continue
        destination.parent.mkdir(parents=True, exist_ok=True)
        with archive.open(info) as source, destination.open("wb") as target:
            shutil.copyfileobj(source, target)
`
	if _, err := s.dockerWithin(
		2*time.Minute,
		"run", "--name", s.trackHelperContainer("backup-restore-extract"),
		"--platform", "linux/amd64",
		"--user", "0:0",
		"--entrypoint", "/usr/local/bin/python",
		"--volume", archivePath+":/tmp/montainer-backup.zip:ro",
		"--volume", worldVolume+":"+containerWorldsRoot,
		"--volume", configVolume+":"+containerConfigsRoot,
		s.suite.legacyImage,
		"-c", restoreScript,
		"/tmp/montainer-backup.zip",
		containerWorldsRoot,
		containerConfigsRoot,
	); err != nil {
		return fmt.Errorf("extract uploaded backup into fresh volumes: %w", err)
	}

	// Exercise the same ownership migration on the externally restored,
	// root-owned archive rather than continuing against the original live
	// volumes. This proves the uploaded LevelDB snapshot itself is usable.
	s.legacyMounts = restoredMounts
	s.candidate = ""
	s.baseURL = ""
	s.udpAddress = ""
	restoredName := strings.Replace(s.networkName, "montainer-real-", "montainer-restored-", 1)
	if err := s.startCandidateContainer(restoredName, s.suite.image); err != nil {
		return fmt.Errorf("start candidate from restored backup: %w", err)
	}
	return nil
}

func (s *scenarioState) customInstancePersistenceOwnedByMontainer() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	return eventually("custom instance persistence to belong to UID/GID 10001", 30*time.Second, func() error {
		_, err := s.dockerWithin(
			10*time.Second,
			"exec", "--user", "10001:10001", s.candidate,
			"/bin/sh", "-ceu",
			`test -s "$1/server.properties"
			test -s "$1/worlds/$5/level.dat"
			test -f "$2/server.properties"
			test -f "$3/legacy-custom-pack.txt"
			test -f "$4/instance.log"
			for root in "$1" "$2" "$3" "$4"; do
				unexpected=$(find "$root" -xdev \( -type d -o -type f -o -type l \) \( ! -uid 10001 -o ! -gid 10001 \) -print -quit)
				test -z "$unexpected" || {
					printf 'unexpected custom-instance owner: %s\n' "$unexpected" >&2
					exit 1
				}
			done`,
			"sh",
			containerCustomRoot,
			containerConfigsRoot,
			containerResourcesRoot,
			containerLogsRoot,
			legacyWorldName,
		)
		return err
	})
}

func (s *scenarioState) nestedMountOwnershipIsPreserved() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	_, err := s.dockerWithin(
		10*time.Second,
		"exec", "--user", "10001:10001", s.candidate,
		"/bin/sh", "-ceu",
		`findmnt -n --mountpoint "$2" >/dev/null
		test "$(stat -c %d "$1")" = "$(stat -c %d "$2")"
		test "$(stat -c %u:%g "$1")" = 10001:10001
		test "$(stat -c %u:%g "$1/$3")" = 10001:10001
		test "$(stat -c %u:%g "$2")" = 0:0
		test "$(stat -c %u:%g "$2/$4")" = 0:0`,
		"sh",
		containerWorldsRoot,
		containerWorldsRoot+"/"+nestedMountDirectory,
		nestedParentCanary,
		nestedMountCanary,
	)
	if err != nil {
		return fmt.Errorf("verify nested mount was pruned from ownership migration: %w", err)
	}
	return nil
}

func (s *scenarioState) candidateContainerEventuallyHealthy() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	return eventually("Docker health probe to report healthy", 75*time.Second, func() error {
		state, err := s.dockerWithin(
			5*time.Second,
			"inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}", s.candidate,
		)
		if err != nil {
			return err
		}
		if state != "healthy" {
			return fmt.Errorf("container health is %q", state)
		}
		return nil
	})
}

func (s *scenarioState) upgradedPersistenceOwnedByMontainer() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	return eventually("all upgraded persistence to belong to UID/GID 10001", 30*time.Second, func() error {
		_, err := s.dockerWithin(
			10*time.Second,
			"exec", "--user", "10001:10001", s.candidate,
			"/bin/sh", "-ceu",
			`test -s "$1/$5/level.dat"
			database_file=$(find "$1/$5/db" -type f -size +0c -print -quit)
			test -n "$database_file"
			test -f "$1/$5/$6"
			test -f "$2/server.properties"
			test -f "$4/instance.log"
			for root in "$1" "$2" "$3" "$4"; do
				unexpected=$(find "$root" -xdev \( -type d -o -type f -o -type l \) \( ! -uid 10001 -o ! -gid 10001 \) -print -quit)
				test -z "$unexpected" || {
					printf 'unexpected upgraded owner: %s\n' "$unexpected" >&2
					exit 1
				}
			done`,
			"sh",
			containerWorldsRoot,
			containerConfigsRoot,
			containerResourcesRoot,
			containerLogsRoot,
			legacyWorldName,
			legacyCanaryName,
		)
		return err
	})
}

func (s *scenarioState) candidateProcessesRunAsMontainer() error {
	if s.candidate == "" {
		return fmt.Errorf("candidate container is not running")
	}
	configuredEnvironment, err := s.dockerWithin(
		10*time.Second,
		"inspect", "--format", "{{range .Config.Env}}{{println .}}{{end}}", s.candidate,
	)
	if err != nil {
		return fmt.Errorf("inspect candidate environment: %w", err)
	}
	for _, variable := range strings.Split(configuredEnvironment, "\n") {
		if strings.HasPrefix(variable, "LD_LIBRARY_PATH=") {
			return fmt.Errorf("LD_LIBRARY_PATH must not be present in the root container environment")
		}
	}
	expectedLibraryPath := containerInstanceRoot
	if configured := strings.TrimSpace(s.env["INSTANCE_DIR"]); configured != "" {
		expectedLibraryPath = configured
	}
	return eventually("Montainer and Bedrock to run as UID/GID 10001", 30*time.Second, func() error {
		status, err := s.currentStatus()
		if err != nil {
			return err
		}
		if status.PID <= 1 {
			return fmt.Errorf("Bedrock PID is %d", status.PID)
		}
		checks := []struct {
			pid  int
			name string
		}{
			{pid: 1, name: "montainer"},
			{pid: status.PID, name: "bedrock_server"},
		}
		for _, check := range checks {
			securityState, err := s.dockerWithin(
				10*time.Second,
				"exec", "--user", "10001:10001", s.candidate,
				"/bin/sh", "-ceu",
				`status="/proc/$1/status"
					test -r "$status"
					test "$(cat "/proc/$1/comm")" = "$2"
					tr '\000' '\n' < "/proc/$1/environ" | grep -Fqx "LD_LIBRARY_PATH=$3"
					awk '
					$1 == "Uid:" {
						uid_seen=1
						for (i=2; i<=5; i++) if ($i != 10001) exit 1
					}
					$1 == "Gid:" {
						gid_seen=1
						for (i=2; i<=5; i++) if ($i != 10001) exit 1
					}
					$1 == "Groups:" {
						groups_seen=1
						for (i=2; i<=NF; i++) if ($i == 0) exit 1
					}
					$1 ~ /^Cap(Inh|Prm|Eff|Bnd|Amb):$/ {
						capabilities_seen++
						if ($2 != "0000000000000000") exit 1
					}
					$1 == "NoNewPrivs:" {
						no_new_privs_seen=1
						if ($2 != 1) exit 1
					}
					END {
						if (!uid_seen || !gid_seen || !groups_seen || capabilities_seen != 5 || !no_new_privs_seen) exit 1
						print "secure"
					}' "$status"`,
				"sh", strconv.Itoa(check.pid), check.name, expectedLibraryPath,
			)
			if err != nil {
				return fmt.Errorf("inspect %s process %d: %w", check.name, check.pid, err)
			}
			if securityState != "secure" {
				return fmt.Errorf("%s process %d security state is %q", check.name, check.pid, securityState)
			}
		}
		return nil
	})
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
		"--user", "10001:10001",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
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
