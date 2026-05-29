package main

import (
	"go.temporal.io/sdk/contrib/aws/lambdaworker"
	"go.temporal.io/sdk/worker"

	"github.com/temporalio/temporal-serverless-no-roads/shared/activities"
	"github.com/temporalio/temporal-serverless-no-roads/shared/taskqueue"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workerconfig"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workflows"
)

func main() {
	// Load concurrency config from environment variables. In Lambda, these are
	// set as function environment variables in the AWS console / Terraform.
	// The .env file in this directory documents the available variables but is
	// NOT bundled into the Lambda zip — Lambda doesn't use .env files.
	cfg := workerconfig.Load()

	lambdaworker.RunWorker(
		worker.WorkerDeploymentVersion{
			DeploymentName: "serverless-webinar",
			BuildID:        "v1.0",
		},
		func(opts *lambdaworker.Options) error {
			opts.TaskQueue = taskqueue.DemoTaskQueue
			opts.WorkerOptions.MaxConcurrentActivityExecutionSize = cfg.MaxConcurrentActivityExecutionSize
			opts.WorkerOptions.MaxConcurrentWorkflowTaskExecutionSize = cfg.MaxConcurrentWorkflowTaskExecutionSize

			opts.RegisterWorkflow(workflows.DemoWorkflow)
			opts.RegisterActivity(&activities.Activities{})

			return nil
		},
	)
}
