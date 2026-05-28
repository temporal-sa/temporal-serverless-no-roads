package main

import (
	"log"

	"github.com/joho/godotenv"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/temporalio/temporal-serverless-no-roads/shared/activities"
	"github.com/temporalio/temporal-serverless-no-roads/shared/taskqueue"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workerconfig"
	"github.com/temporalio/temporal-serverless-no-roads/shared/workflows"
)

func main() {
	// Load .env if present — silently ignored if missing (e.g. env vars are
	// already set in the shell or in a container environment).
	if err := godotenv.Load(); err != nil {
		log.Println("localworker: no .env file found, using environment variables")
	}

	cfg := workerconfig.Load()

	c, err := client.Dial(client.Options{
		// Defaults to localhost:7233 / namespace "default" — matches
		// what `temporal server start-dev` provides out of the box.
	})
	if err != nil {
		log.Fatalln("failed to create Temporal client:", err)
	}
	defer c.Close()

	w := worker.New(c, taskqueue.DemoTaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: cfg.MaxConcurrentWorkflowTaskExecutionSize,
	})
	w.RegisterWorkflow(workflows.DemoWorkflow)
	w.RegisterActivity(&activities.Activities{})

	log.Printf("local worker started, polling task queue: %s", taskqueue.DemoTaskQueue)

	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalln("worker error:", err)
	}
}
