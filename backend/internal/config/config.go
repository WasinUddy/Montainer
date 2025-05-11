package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds application configuration
type Config struct {
	// Subpath URL
	SubpathURL string

	// Backup S3 settings
	AWSS3Endpoint   string
	AWSS3KeyID      string
	AWSS3SecretKey  string
	AWSS3BucketName string
	AWSS3Region     string

	// Session specific settings
	InstanceName string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	godotenv.Load()

	return &Config{
		SubpathURL:      getEnv("SUBPATH_URL", "/"),
		AWSS3Endpoint:   getEnv("AWS_S3_ENDPOINT", ""),
		AWSS3KeyID:      getEnv("AWS_S3_KEY_ID", ""),
		AWSS3SecretKey:  getEnv("AWS_S3_SECRET_KEY", ""),
		AWSS3BucketName: getEnv("AWS_S3_BUCKET_NAME", ""),
		AWSS3Region:     getEnv("AWS_S3_REGION", ""),
		InstanceName:    getEnv("INSTANCE_NAME", "Montainer"),
	}, nil
}

// Helper function to get environment variables with default values
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && strings.TrimSpace(value) != "" {
		return value
	}
	return defaultValue
}
