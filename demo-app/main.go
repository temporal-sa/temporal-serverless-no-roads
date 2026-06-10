package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/temporalio/temporal-serverless-no-roads/demo-app/api"
	"github.com/temporalio/temporal-serverless-no-roads/demo-app/cache"
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

	// --- Metrics cache: 2 second TTL ---
	// Lambda concurrency is now sourced from Temporal's live poller count
	// (via DescribeTaskQueue) rather than CloudWatch, so a 2s TTL is
	// appropriate — all metrics are now in the same freshness tier.
	metricsCache := cache.NewMetricsCache(2 * time.Second)

	// --- Routes ---
	mux := http.NewServeMux()

	// Audience submission
	mux.HandleFunc("/api/submit", api.SubmitHandler(temporalClient))

	// Presenter burst seeding — no rate limit, presenter use only
	// POST /api/seed          → starts 30 workflows
	// POST /api/seed?count=N  → starts N workflows (max 200)
	mux.HandleFunc("/api/seed", api.SeedHandler(temporalClient))

	// Metrics polling endpoint
	mux.HandleFunc("/api/metrics", api.MetricsHandler(
		temporalClient, metricsCache,
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
