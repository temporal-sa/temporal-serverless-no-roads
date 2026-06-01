package workerconfig

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/joho/godotenv"
	"go.temporal.io/sdk/client"
)

func init() {
	// Resolve the directory that contains this source file so the .env can
	// live next to workerconfig.go regardless of where the binary is invoked.
	// runtime.Caller embeds the compile-time path, which is always valid in a
	// `go run` / local-dev workflow. In containerised or Lambda deployments the
	// env vars are injected directly and no .env file is expected.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	envPath := filepath.Join(filepath.Dir(file), ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("workerconfig: no .env loaded from %s (using environment variables)", envPath)
	}
}

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

// BuildClientOptions returns client.Options populated from environment
// variables:
//
//	TEMPORAL_ADDRESS   — gRPC endpoint (e.g. <ns>.<acct>.tmprl.cloud:7233)
//	TEMPORAL_NAMESPACE — Temporal namespace
//	TEMPORAL_API_KEY   — API key; if set, credentials + TLS are enabled
//
// When the env vars are absent the options default to localhost:7233 /
// "default" namespace with no auth, matching `temporal server start-dev`.
func BuildClientOptions() client.Options {
	opts := client.Options{}

	if addr := os.Getenv("TEMPORAL_ADDRESS"); addr != "" {
		opts.HostPort = addr
	}
	if ns := os.Getenv("TEMPORAL_NAMESPACE"); ns != "" {
		opts.Namespace = ns
	}

	apiKey := os.Getenv("TEMPORAL_API_KEY")
	if apiKey == "" {
		log.Println("workerconfig: TEMPORAL_API_KEY not set, connecting without credentials (local dev)")
		return opts
	}
	opts.Credentials = client.NewAPIKeyStaticCredentials(apiKey)
	return opts
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
