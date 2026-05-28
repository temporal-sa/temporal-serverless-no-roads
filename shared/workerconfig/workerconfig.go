package workerconfig

import (
	"log"
	"os"
	"strconv"
)

// Defaults — intentionally low to make backlog and sync match rate pressure
// visible during the demo. Set higher for production Lambda deployments.
const (
	DefaultMaxConcurrentActivity = 5
	DefaultMaxConcurrentWorkflow = 5
)

// Config holds worker concurrency settings loaded from environment variables.
type Config struct {
	MaxConcurrentActivityExecutionSize     int
	MaxConcurrentWorkflowTaskExecutionSize int
}

// Load reads worker concurrency settings from environment variables, falling
// back to defaults if unset. Logs the values in use at startup.
//
// Environment variables:
//
//	WORKER_MAX_CONCURRENT_ACTIVITIES  (default: 5)
//	WORKER_MAX_CONCURRENT_WORKFLOWS   (default: 5)
func Load() Config {
	cfg := Config{
		MaxConcurrentActivityExecutionSize:     envInt("WORKER_MAX_CONCURRENT_ACTIVITIES", DefaultMaxConcurrentActivity),
		MaxConcurrentWorkflowTaskExecutionSize: envInt("WORKER_MAX_CONCURRENT_WORKFLOWS", DefaultMaxConcurrentWorkflow),
	}
	log.Printf("worker config: max_concurrent_activities=%d max_concurrent_workflows=%d",
		cfg.MaxConcurrentActivityExecutionSize,
		cfg.MaxConcurrentWorkflowTaskExecutionSize,
	)
	return cfg
}

// envInt reads an integer from an environment variable, returning defaultVal
// if the variable is unset or unparseable.
func envInt(key string, defaultVal int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("workerconfig: invalid value for %s=%q, using default %d", key, raw, defaultVal)
		return defaultVal
	}
	if v < 1 {
		log.Printf("workerconfig: %s=%d is < 1, using default %d", key, v, defaultVal)
		return defaultVal
	}
	return v
}
