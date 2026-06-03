package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"

	"github.com/temporalio/temporal-serverless-no-roads/demo-app/cache"
	"github.com/temporalio/temporal-serverless-no-roads/shared/taskqueue"
)

// MetricsResponse is the JSON shape the frontend polls for.
type MetricsResponse struct {
	RunningWorkflows   int64   `json:"runningWorkflows"`
	CompletedWorkflows int64   `json:"completedWorkflows"`
	// LambdaConcurrency is the number of active Lambda invocations, derived
	// from the count of live pollers on the activity task queue. Each Lambda
	// invocation registers exactly one activity-task poller, so this count is
	// real-time (seconds of lag) rather than relying on CloudWatch (1-3 min lag).
	LambdaConcurrency  float64 `json:"lambdaConcurrency"`
	BacklogDepth       float64 `json:"backlogDepth"`
	// SyncMatchRate is the percentage of tasks that were immediately dispatched
	// to a polling worker (sync-matched) vs. having to wait in the backlog.
	// 100% = every task found a waiting worker instantly, no scaling needed.
	// As it drops, tasks are arriving faster than workers can consume them —
	// this is the primary signal Temporal uses to trigger new Lambda invocations.
	// -1 signals "no data yet" (no tasks have been dispatched in the window).
	SyncMatchRate float64 `json:"syncMatchRate"`
}

// MetricsHandler fans out to Temporal concurrently, merges results, and
// returns JSON. Responses are cached for a short TTL to avoid hammering the
// Temporal APIs when many browser tabs are polling simultaneously.
func MetricsHandler(
	tc client.Client,
	metricsCache *cache.MetricsCache,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		// Return cached response if still fresh.
		if cached, ok := metricsCache.Get(); ok {
			w.Write(cached)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		var (
			wg       sync.WaitGroup
			mu       sync.Mutex
			response MetricsResponse
		)

		// Initialise SyncMatchRate to -1 so the frontend can distinguish
		// "not yet measured" from a genuine 0%.
		response.SyncMatchRate = -1

		// Fan-out 1: Temporal — running and completed workflow counts.
		wg.Add(1)
		go func() {
			defer wg.Done()
			running, completed, err := fetchTemporalWorkflowCounts(ctx, tc)
			if err != nil {
				log.Printf("metrics: fetchTemporalWorkflowCounts: %v", err)
				return
			}
			mu.Lock()
			response.RunningWorkflows = running
			response.CompletedWorkflows = completed
			mu.Unlock()
		}()

		// Fan-out 2: Temporal — task queue stats (backlog depth, sync match rate,
		// and active Lambda poller count). Uses two DescribeTaskQueue calls
		// (workflow + activity) with ReportStats: true.
		wg.Add(1)
		go func() {
			defer wg.Done()
			backlog, syncMatchRate, pollerCount, err := fetchTaskQueueStats(ctx, tc)
			if err != nil {
				log.Printf("metrics: fetchTaskQueueStats: %v", err)
				return
			}
			mu.Lock()
			response.BacklogDepth = backlog
			response.SyncMatchRate = syncMatchRate
			response.LambdaConcurrency = float64(pollerCount)
			mu.Unlock()
		}()

		wg.Wait()

		data, _ := json.Marshal(response)
		metricsCache.Set(data)
		w.Write(data)
	}
}

// fetchTemporalWorkflowCounts queries Temporal for running and completed
// workflow counts on the demo task queue.
func fetchTemporalWorkflowCounts(ctx context.Context, tc client.Client) (running, completed int64, err error) {
	runningResp, err := tc.CountWorkflow(ctx, &workflowservice.CountWorkflowExecutionsRequest{
		Query: `TaskQueue="` + taskqueue.DemoTaskQueue + `" AND ExecutionStatus="Running"`,
	})
	if err != nil {
		return 0, 0, err
	}

	completedResp, err := tc.CountWorkflow(ctx, &workflowservice.CountWorkflowExecutionsRequest{
		Query: `TaskQueue="` + taskqueue.DemoTaskQueue + `" AND ExecutionStatus="Completed"`,
	})
	if err != nil {
		return 0, 0, err
	}

	return runningResp.Count, completedResp.Count, nil
}

// fetchTaskQueueStats returns backlog depth, sync match rate, and the number
// of active Lambda worker pollers for the demo task queue. It calls the
// standard DescribeTaskQueue gRPC endpoint with ReportStats: true — once for
// workflow tasks and once for activity tasks.
//
// Using the standard (non-enhanced) DescribeTaskQueue rather than the
// deprecated DescribeTaskQueueEnhanced is intentional: the enhanced API's
// VersionsInfo is tied to the old Build-ID versioning model and returns empty
// stats for workers that use the newer Worker Deployment versioning
// (DeploymentOptions / UseVersioning: true). The standard endpoint returns
// aggregate TaskQueueStats regardless of which versioning model the workers use.
//
// Sync match rate: the percentage of tasks dispatched immediately to a polling
// worker (sync-matched) rather than persisted to the backlog first.
//
//	syncMatchRate = 1 - ((TasksAddRate - TasksDispatchRate) / TasksAddRate)
//
// -1 is returned when TasksAddRate is 0 so the frontend can show "—" instead
// of 0% or 100% while the queue is idle.
//
// Poller count: the number of active pollers on the activity task queue. Each
// Lambda invocation registers exactly one activity-task poller, so this is a
// real-time proxy for Lambda concurrency — far fresher than CloudWatch metrics.
func fetchTaskQueueStats(ctx context.Context, tc client.Client) (backlog float64, syncMatchRate float64, pollerCount int, err error) {
	namespace := os.Getenv("TEMPORAL_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	tqTypes := []enumspb.TaskQueueType{
		enumspb.TASK_QUEUE_TYPE_WORKFLOW,
		enumspb.TASK_QUEUE_TYPE_ACTIVITY,
	}

	var (
		totalBacklog           int64
		totalTasksAddRate      float32
		totalTasksDispatchRate float32
	)

	for _, tqType := range tqTypes {
		resp, respErr := tc.WorkflowService().DescribeTaskQueue(ctx, &workflowservice.DescribeTaskQueueRequest{
			Namespace: namespace,
			TaskQueue: &taskqueuepb.TaskQueue{
				Name: taskqueue.DemoTaskQueue,
			},
			TaskQueueType: tqType,
			ReportStats:   true,
		})
		if respErr != nil {
			return 0, -1, 0, respErr
		}
		if resp.Stats != nil {
			log.Printf("fetchTaskQueueStats: type=%v backlog=%d addRate=%f dispatchRate=%f pollers=%d",
				tqType,
				resp.Stats.ApproximateBacklogCount,
				resp.Stats.TasksAddRate,
				resp.Stats.TasksDispatchRate,
				len(resp.Pollers),
			)
			totalBacklog += resp.Stats.ApproximateBacklogCount
			totalTasksAddRate += resp.Stats.TasksAddRate
			totalTasksDispatchRate += resp.Stats.TasksDispatchRate
		} else {
			log.Printf("fetchTaskQueueStats: type=%v stats nil", tqType)
		}

		// Count active Lambda invocations via activity-task pollers.
		// Each Lambda invocation = exactly one activity-task poller connection.
		if tqType == enumspb.TASK_QUEUE_TYPE_ACTIVITY {
			pollerCount = len(resp.Pollers)
		}
	}

	backlog = float64(totalBacklog)

	// Can't compute a meaningful rate if no tasks have been added yet.
	if totalTasksAddRate <= 0 {
		return backlog, -1, pollerCount, nil
	}

	// syncMatchRate = fraction of tasks that did NOT contribute to backlog growth.
	// BacklogIncreaseRate = TasksAddRate - TasksDispatchRate
	// Clamp to [0, 100] to guard against transient metric noise.
	totalBacklogIncRate := totalTasksAddRate - totalTasksDispatchRate
	rate := (1.0 - float64(totalBacklogIncRate)/float64(totalTasksAddRate)) * 100.0
	if rate > 100.0 {
		rate = 100.0
	}
	if rate < 0.0 {
		rate = 0.0
	}

	return backlog, rate, pollerCount, nil
}
