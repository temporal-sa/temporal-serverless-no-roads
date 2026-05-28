package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"go.temporal.io/sdk/client"

	"github.com/temporalio/temporal-serverless-no-roads/shared/taskqueue"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workflows"
)

const (
	seedDefaultCount = 30
	seedMaxCount     = 200
)

// SeedResponse summarises the result of a seed burst.
type SeedResponse struct {
	Requested int      `json:"requested"`
	Started   int      `json:"started"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

// SeedHandler fires a configurable burst of DemoWorkflows simultaneously.
// Intended for presenter use to prime the scaling visual before or during
// the live demo, regardless of audience size.
//
// Usage:
//
//	POST /api/seed          — starts seedDefaultCount workflows
//	POST /api/seed?count=50 — starts 50 workflows (max seedMaxCount)
//
// Workflows are started concurrently so the burst hits the task queue all
// at once, maximising the backlog spike visible on the dashboard.
func SeedHandler(tc client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		count := seedDefaultCount
		if raw := r.URL.Query().Get("count"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				http.Error(w, "count must be a positive integer", http.StatusBadRequest)
				return
			}
			if n > seedMaxCount {
				n = seedMaxCount
			}
			count = n
		}

		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			started int
			errs    []string
		)

		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				name := fmt.Sprintf("seed-%03d", i+1)
				workflowID := "demo-seed-" + shortID()

				opts := client.StartWorkflowOptions{
					ID:        workflowID,
					TaskQueue: taskqueue.DemoTaskQueue,
				}

				_, err := tc.ExecuteWorkflow(r.Context(), opts, workflows.DemoWorkflow, workflows.DemoInput{
					Name: name,
				})

				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", workflowID, err))
				} else {
					started++
				}
			}(i)
		}

		wg.Wait()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SeedResponse{
			Requested: count,
			Started:   started,
			Failed:    len(errs),
			Errors:    errs,
		})
	}
}
