package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"go.temporal.io/sdk/client"

	"github.com/temporalio/temporal-serverless-no-roads/demo-app/api"
	"github.com/temporalio/temporal-serverless-no-roads/demo-app/cache"
	"github.com/temporalio/temporal-serverless-no-roads/demo-app/middleware"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workerconfig"
)

//go:embed frontend/*
var frontendFS embed.FS

func main() {
	// --- Temporal client ---
	// Reads TEMPORAL_ADDRESS, TEMPORAL_NAMESPACE, and TEMPORAL_API_KEY from
	// environment variables. Falls back to localhost:7233 / default namespace
	// with no auth when env vars are absent (local dev).
	temporalClient, err := client.Dial(workerconfig.BuildClientOptions())
	if err != nil {
		log.Fatalf("failed to create Temporal client: %v", err)
	}
	defer temporalClient.Close()

	// --- AWS CloudWatch client (uses IRSA in EKS — no static creds needed) ---
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	cwClient := cloudwatch.NewFromConfig(awsCfg)

	// --- Lambda function name (set via env var in k8s deployment) ---
	lambdaFunctionName := os.Getenv("LAMBDA_FUNCTION_NAME")
	if lambdaFunctionName == "" {
		log.Fatal("LAMBDA_FUNCTION_NAME env var is required")
	}

	// --- Metrics cache: 3 second TTL ---
	metricsCache := cache.NewMetricsCache(3 * time.Second)

	// --- Routes ---
	mux := http.NewServeMux()

	// Audience submission — rate limited per IP
	mux.Handle("/api/submit", middleware.RateLimit(
		http.HandlerFunc(api.SubmitHandler(temporalClient)),
	))

	// Presenter burst seeding — no rate limit, presenter use only
	// POST /api/seed          → starts 30 workflows
	// POST /api/seed?count=N  → starts N workflows (max 200)
	mux.HandleFunc("/api/seed", api.SeedHandler(temporalClient))

	// Metrics polling endpoint
	mux.HandleFunc("/api/metrics", api.MetricsHandler(
		temporalClient, cwClient, metricsCache, lambdaFunctionName,
	))

	// Serve embedded frontend — strip the "frontend/" prefix from embed paths
	stripped, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("failed to sub frontend fs: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(stripped)))

	// --- Start server ---
	addr := ":8080"
	log.Printf("demo-app listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
