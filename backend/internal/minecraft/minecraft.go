package minecraft

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// MinecraftServer represents a Minecraft Bedrock Edition server instance
type MinecraftServer struct {
	cwd         string
	configFiles []string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	logFile     *os.File
	isRunning   bool
	s3Client    *s3.Client
	config      *ServerConfig
	mu          sync.Mutex // For thread safety
}

// ServerConfig holds the S3 configuration from settings
type ServerConfig struct {
	AWSS3Endpoint   string
	AWSS3KeyID      string
	AWSS3SecretKey  string
	AWSS3BucketName string
	AWSS3Region     string
	InstanceName    string
}

// NewMinecraftServer creates and initializes a new MinecraftServer instance
func NewMinecraftServer(cwd string, config *ServerConfig) *MinecraftServer {
	server := &MinecraftServer{
		cwd:         cwd,
		configFiles: []string{"server.properties", "allowlist.json", "permissions.json"},
		isRunning:   false,
		config:      config,
	}

	// Initialize S3 client if configured
	if config.AWSS3Endpoint != "" {
		server.initS3Client()
	}

	// Auto-start the server
	err := server.Start()
	if err != nil {
		fmt.Printf("Error starting Minecraft server: %v\n", err)
	}

	return server
}

// initS3Client initializes the S3 client for backup operations
func (m *MinecraftServer) initS3Client() error {
	// Create custom resolver for S3-compatible endpoint
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               m.config.AWSS3Endpoint,
			HostnameImmutable: true,
			SigningRegion:     m.config.AWSS3Region,
		}, nil
	})

	// Configure AWS SDK with custom credentials and endpoint
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(m.config.AWSS3Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			m.config.AWSS3KeyID,
			m.config.AWSS3SecretKey,
			"",
		)),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	m.s3Client = s3.NewFromConfig(cfg)
	return nil
}

// IsRunning returns whether the server is currently running
func (m *MinecraftServer) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isRunning
}

// Start starts the Minecraft server instance if it's not already running
func (m *MinecraftServer) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return nil // Server is already running
	}

	// Sync config files from mapped volume to game instance directory
	for _, configFile := range m.configFiles {
		configPath := filepath.Join("./configs", configFile)
		instancePath := filepath.Join(m.cwd, configFile)

		// Check if config file exists, if not, copy from instance
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if err := copyFile(instancePath, configPath); err != nil {
				return fmt.Errorf("error copying config file %s: %w", configFile, err)
			}
		} else {
			// Copy from configs to instance
			if err := copyFile(configPath, instancePath); err != nil {
				return fmt.Errorf("error copying config file %s: %w", configFile, err)
			}
		}
	}

	// Create or truncate log file
	logFile, err := os.OpenFile("instance.log", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}
	m.logFile = logFile

	// Start the server process
	m.cmd = exec.Command("./bedrock_server")
	m.cmd.Dir = m.cwd
	m.cmd.Stdout = logFile
	m.cmd.Stderr = logFile

	stdin, err := m.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}
	m.stdin = stdin

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("error starting server process: %w", err)
	}

	m.isRunning = true

	// Start a goroutine to monitor the process and update isRunning when it exits
	go func() {
		m.cmd.Wait()

		m.mu.Lock()
		m.isRunning = false
		m.logFile.Close()
		m.mu.Unlock()
	}()

	return nil
}

// Stop stops the Minecraft server if it's running
func (m *MinecraftServer) Stop(forceShutdown bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return nil // Server is not running
	}

	if m.cmd != nil && m.cmd.Process != nil {
		if forceShutdown {
			// Force kill the process
			if err := m.cmd.Process.Kill(); err != nil {
				return fmt.Errorf("error killing server process: %w", err)
			}
		} else {
			// Send stop command
			if _, err := m.stdin.Write([]byte("stop\n")); err != nil {
				return fmt.Errorf("error sending stop command: %w", err)
			}
		}

		// Close log file
		if m.logFile != nil {
			m.logFile.Close()
			m.logFile = nil
		}

		// Remove the log file
		os.Remove("instance.log")

		// Mark as not running
		m.isRunning = false
		m.cmd = nil
		m.stdin = nil
	}

	return nil
}

// SendCommand sends a command to the Minecraft server
func (m *MinecraftServer) SendCommand(commandString string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning || m.stdin == nil {
		return fmt.Errorf("server instance is not running")
	}

	if _, err := m.stdin.Write([]byte(commandString + "\n")); err != nil {
		return fmt.Errorf("error sending command: %w", err)
	}

	// Handle the stop command specially
	if commandString == "stop" {
		if m.logFile != nil {
			m.logFile.Close()
			m.logFile = nil
		}
		os.Remove("instance.log")
		m.isRunning = false
		m.cmd = nil
		m.stdin = nil
	}

	return nil
}

// SaveData saves the server data to S3
func (m *MinecraftServer) SaveData() error {
	if m.config.AWSS3Endpoint == "" {
		return fmt.Errorf("AWS S3 settings not configured")
	}

	// Stop the server first
	if err := m.Stop(false); err != nil {
		return fmt.Errorf("error stopping server: %w", err)
	}

	tmpDir := "./tmp"
	zipFile := "./tmp.zip"

	// Clean up any existing temporary files
	os.RemoveAll(tmpDir)
	os.Remove(zipFile)

	// Create fresh temporary directory
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}

	defer func() {
		// Clean up temporary files
		os.RemoveAll(tmpDir)
		os.Remove(zipFile)

		// Restart the server
		m.Start()
	}()

	// Copy the world data to temporary directory
	worldsSrc := filepath.Join(m.cwd, "worlds")
	worldsDest := filepath.Join(tmpDir, "worlds")
	if err := copyDir(worldsSrc, worldsDest); err != nil {
		return fmt.Errorf("error copying world data: %w", err)
	}

	// Copy config files
	for _, configFile := range m.configFiles {
		src := filepath.Join("./configs", configFile)
		dst := filepath.Join(tmpDir, configFile)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("error copying config file %s: %w", configFile, err)
		}
	}

	// Create zip archive
	if err := createZipArchive(tmpDir, zipFile); err != nil {
		return fmt.Errorf("error creating zip archive: %w", err)
	}

	// Verify S3 bucket exists
	_, err := m.s3Client.HeadBucket(context.Background(), &s3.HeadBucketInput{
		Bucket: aws.String(m.config.AWSS3BucketName),
	})
	if err != nil {
		return fmt.Errorf("S3 bucket '%s' is not accessible: %w", m.config.AWSS3BucketName, err)
	}

	// Upload to S3
	backupKey := fmt.Sprintf("%s_%d_backup.zip", m.config.InstanceName, time.Now().Unix())
	file, err := os.Open(zipFile)
	if err != nil {
		return fmt.Errorf("error opening zip file for upload: %w", err)
	}
	defer file.Close()

	_, err = m.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(m.config.AWSS3BucketName),
		Key:    aws.String(backupKey),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("error uploading backup to S3: %w", err)
	}

	return nil
}

// Helper function to copy a file
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

// Helper function to copy a directory
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// Helper function to create a zip archive
func createZipArchive(sourceDir, zipFile string) error {
	// Execute zip command as a subprocess
	cmd := exec.Command("zip", "-r", zipFile, ".")
	cmd.Dir = sourceDir
	return cmd.Run()
}
