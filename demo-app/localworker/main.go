package main

import (
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/temporalio/temporal-serverless-no-roads/shared/activities"
	"github.com/temporalio/temporal-serverless-no-roads/shared/taskqueue"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workerconfig"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workflows"
)

func main() {
	// workerconfig's init() automatically loads shared/workerconfig/.env via
	// runtime.Caller, so no explicit env-file handling is needed here.
	// In containerised or Lambda deployments the env vars are injected
	// directly and no .env file is expected.
	cfg := workerconfig.Load()

	c, err := client.Dial(workerconfig.BuildClientOptions())
	if err != nil {
		log.Fatalln("failed to create Temporal client:", err)
	}
	defer c.Close()

	w := worker.New(c, taskqueue.DemoTaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: cfg.MaxConcurrentWorkflowTaskExecutionSize,
		// Match the versioning config used by the Lambda worker so local dev
		// behaviour is consistent with production.
		DeploymentOptions: worker.DeploymentOptions{
			UseVersioning:             true,
			DefaultVersioningBehavior: workflow.VersioningBehaviorPinned,
			Version: worker.WorkerDeploymentVersion{
				DeploymentName: "serverless-webinar",
				BuildID:        "1.0.0",
			},
		},
	})
	w.RegisterWorkflow(workflows.DemoWorkflow)
	w.RegisterActivity(&activities.Activities{})

	log.Printf("local worker started, polling task queue: %s", taskqueue.DemoTaskQueue)

	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalln("worker error:", err)
	}
}
